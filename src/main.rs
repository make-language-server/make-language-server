use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::io::{self, BufRead, Write};
use std::ops::Range;
use streaming_iterator::StreamingIterator;

#[derive(Debug, Clone)]
struct CapturedNode {
    byte_range: Range<usize>,
    start_row: usize,
    start_col: usize,
    end_row: usize,
    end_col: usize,
}

fn capture_nodes(text: &[u8]) -> Vec<CapturedNode> {
    let mut parser = tree_sitter::Parser::new();
    parser
        .set_language(&tree_sitter_make::LANGUAGE.into())
        .expect("failed to set language");
    let tree = parser.parse(text, None).expect("failed to parse");
    let root = tree.root_node();
    let query = tree_sitter::Query::new(
        &tree_sitter_make::LANGUAGE.into(),
        r#"
        (define_directive name: (word) @define_directive_name)
        (variable_assignment name: (word) @variable_assignment_name)
        "#,
    )
    .expect("failed to create query");
    let mut cursor = tree_sitter::QueryCursor::new();
    let mut matches = cursor.matches(&query, root, text);
    let mut nodes = Vec::new();
    while let Some(m) = matches.next() {
        for capture in m.captures.iter() {
            let node = capture.node;
            nodes.push(CapturedNode {
                byte_range: node.start_byte()..node.end_byte(),
                start_row: node.start_position().row,
                start_col: node.start_position().column,
                end_row: node.end_position().row,
                end_col: node.end_position().column,
            });
        }
    }
    nodes
}

fn capture_completions(text: &[u8]) -> Vec<String> {
    capture_nodes(text)
        .iter()
        .map(|node| String::from_utf8_lossy(&text[node.byte_range.clone()]).into_owned())
        .collect()
}

#[derive(Debug, Clone, Copy, Serialize)]
struct LspPosition {
    character: u32,
    line: u32,
}

#[derive(Debug, Clone, Copy, Serialize)]
struct LspRange {
    start: LspPosition,
    end: LspPosition,
}

fn get_definition_range(text: &[u8], word: &str) -> Option<LspRange> {
    capture_nodes(text).iter().find_map(|node| {
        let node_text = &text[node.byte_range.clone()];
        if node_text == word.as_bytes() {
            Some(LspRange {
                start: LspPosition {
                    character: node.start_col as u32,
                    line: node.start_row as u32,
                },
                end: LspPosition {
                    character: node.end_col as u32,
                    line: node.end_row as u32,
                },
            })
        } else {
            None
        }
    })
}

#[derive(Deserialize)]
struct Request {
    id: Option<Value>,
    method: String,
    params: Option<Value>,
}

#[derive(Deserialize)]
struct TextDocumentIdentifier {
    uri: String,
}

#[derive(Deserialize)]
struct TextDocumentItem {
    uri: String,
    text: String,
}

#[derive(Deserialize)]
struct VersionedTextDocumentIdentifier {
    uri: String,
}

#[derive(Deserialize)]
struct DidOpenTextDocumentParams {
    #[serde(rename = "textDocument")]
    text_document: TextDocumentItem,
}

#[derive(Deserialize)]
struct DidCloseTextDocumentParams {
    #[serde(rename = "textDocument")]
    text_document: TextDocumentIdentifier,
}

#[derive(Deserialize)]
struct ContentChange {
    text: String,
}

#[derive(Deserialize)]
struct DidChangeTextDocumentParams {
    #[serde(rename = "textDocument")]
    text_document: VersionedTextDocumentIdentifier,
    #[serde(rename = "contentChanges")]
    content_changes: Vec<ContentChange>,
}

#[derive(Deserialize)]
struct TextDocumentPositionParams {
    #[serde(rename = "textDocument")]
    text_document: TextDocumentIdentifier,
    position: PositionParam,
}

#[derive(Deserialize)]
struct PositionParam {
    line: u32,
    character: u32,
}

fn read_message(stdin: &mut impl BufRead) -> io::Result<Option<String>> {
    let mut content_length: usize = 0;
    loop {
        let mut header = String::new();
        if stdin.read_line(&mut header)? == 0 {
            return Ok(None);
        }
        let header = header.trim();
        if header.is_empty() {
            break;
        }
        if let Some(len) = header.strip_prefix("Content-Length: ") {
            content_length = len.parse().unwrap_or(0);
        }
    }
    if content_length == 0 {
        return Ok(None);
    }
    let mut body = vec![0u8; content_length];
    stdin.read_exact(&mut body)?;
    Ok(String::from_utf8(body).ok())
}

fn send_message(stdout: &mut impl Write, body: Value) -> io::Result<()> {
    let payload = serde_json::to_string(&body).expect("failed to serialize response");
    write!(stdout, "Content-Length: {}\r\n\r\n{}", payload.len(), payload)?;
    stdout.flush()
}

fn send_response(stdout: &mut impl Write, id: &Value, result: Value) -> io::Result<()> {
    send_message(
        stdout,
        serde_json::json!({
            "id": id,
            "result": result,
            "jsonrpc": "2.0"
        }),
    )
}

fn send_error(stdout: &mut impl Write, id: &Value, code: i64, message: &str) -> io::Result<()> {
    send_message(
        stdout,
        serde_json::json!({
            "id": id,
            "error": { "code": code, "message": message },
            "jsonrpc": "2.0"
        }),
    )
}

fn extract_word_at(line: &str, character: usize) -> Option<(Option<char>, &str)> {
    let is_delimiter = |ch: char| matches!(ch, ' ' | ',' | '(' | ')' | '$');

    let mut begin = None;
    let mut left_stop = None;

    for (i, ch) in line.char_indices().rev().skip(line.len() - character - 1) {
        if is_delimiter(ch) {
            left_stop = Some(ch);
            begin = Some(i + ch.len_utf8());
            break;
        }
    }

    let mut end = None;
    for (i, ch) in line.char_indices().skip(character) {
        if matches!(ch, ' ' | ',' | '(' | ')') {
            end = Some(i);
            break;
        }
    }

    if left_stop == Some('$') && end.is_none() {
        end = begin.map(|b| b + 1);
    }

    let begin = begin?;
    let end = end?;
    Some((left_stop, &line[begin..end]))
}

fn main() {
    let mut documents: HashMap<String, String> = HashMap::new();
    let mut stdin = io::stdin().lock();
    let mut stdout = io::stdout().lock();

    loop {
        let message = match read_message(&mut stdin) {
            Ok(Some(msg)) => msg,
            Ok(None) => break,
            Err(e) => {
                eprintln!("failed to read message: {e}");
                break;
            }
        };

        let request: Request = match serde_json::from_str(&message) {
            Ok(r) => r,
            Err(_) => continue,
        };

        match request.method.as_str() {
            "initialize" => {
                let id = match &request.id {
                    Some(id) => id,
                    None => continue,
                };
                let _ = send_response(
                    &mut stdout,
                    id,
                    serde_json::json!({
                        "capabilities": {
                            "completionProvider": {},
                            "definitionProvider": true,
                            "textDocumentSync": 1
                        }
                    }),
                );
            }
            "shutdown" => {
                if let Some(id) = &request.id {
                    let _ = send_response(&mut stdout, id, Value::Null);
                }
                break;
            }
            "textDocument/completion" => {
                let id = match &request.id {
                    Some(id) => id,
                    None => continue,
                };
                let params: TextDocumentPositionParams =
                    match serde_json::from_value(request.params.unwrap_or(Value::Null)) {
                        Ok(p) => p,
                        Err(_) => {
                            let _ = send_error(&mut stdout, id, -32602, "invalid params");
                            continue;
                        }
                    };
                let empty = String::new();
                let text = documents.get(&params.text_document.uri).unwrap_or(&empty);
                let completions = capture_completions(text.as_bytes());
                let items: Vec<Value> = completions
                    .iter()
                    .map(|c| serde_json::json!({"label": c, "insertText": c}))
                    .collect();
                let _ = send_response(&mut stdout, id, Value::Array(items));
            }
            "textDocument/definition" => {
                let id = match &request.id {
                    Some(id) => id,
                    None => continue,
                };
                let params: TextDocumentPositionParams =
                    match serde_json::from_value(request.params.unwrap_or(Value::Null)) {
                        Ok(p) => p,
                        Err(_) => {
                            let _ = send_error(&mut stdout, id, -32602, "invalid params");
                            continue;
                        }
                    };
                let empty = String::new();
                let text = documents.get(&params.text_document.uri).unwrap_or(&empty);
                let lines: Vec<&str> = text.split('\n').collect();
                let line = match lines.get(params.position.line as usize) {
                    Some(l) => *l,
                    None => {
                        let _ = send_error(&mut stdout, id, -32602, "line out of range");
                        continue;
                    }
                };
                let character = params.position.character as usize;

                let word = match extract_word_at(line, character) {
                    Some((_, w)) => w,
                    None => {
                        let _ = send_error(&mut stdout, id, -32602, "failed reading symbol");
                        continue;
                    }
                };

                match get_definition_range(text.as_bytes(), word) {
                    Some(range) => {
                        let _ = send_response(
                            &mut stdout,
                            id,
                            serde_json::json!({
                                "uri": params.text_document.uri,
                                "range": range
                            }),
                        );
                    }
                    None => {
                        let _ = send_error(&mut stdout, id, -32602, "definition not found");
                    }
                }
            }
            "textDocument/didOpen" => {
                if let Ok(params) = serde_json::from_value::<DidOpenTextDocumentParams>(
                    request.params.unwrap_or(Value::Null),
                ) {
                    documents.insert(params.text_document.uri, params.text_document.text);
                }
            }
            "textDocument/didClose" => {
                if let Ok(params) = serde_json::from_value::<DidCloseTextDocumentParams>(
                    request.params.unwrap_or(Value::Null),
                ) {
                    documents.remove(&params.text_document.uri);
                }
            }
            "textDocument/didChange" => {
                if let Ok(params) = serde_json::from_value::<DidChangeTextDocumentParams>(
                    request.params.unwrap_or(Value::Null),
                ) {
                    // LSP spec: last change is the full document text when sync mode is Full
                    if let Some(change) = params.content_changes.into_iter().last() {
                        documents.insert(params.text_document.uri, change.text);
                    }
                }
            }
            _ => {}
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_capture_completions() {
        let makefile = std::fs::read("testdata/captureCompletions.mk").unwrap();
        let completions = capture_completions(&makefile);
        assert_eq!(
            completions,
            vec!["snake_glue_function", "glued_variable", "target_echo_template"]
        );
    }

    #[test]
    fn test_get_definition_range() {
        let makefile = std::fs::read("testdata/captureCompletions.mk").unwrap();
        let range = get_definition_range(&makefile, "target_echo_template").unwrap();
        assert_eq!(range.start.character, 7);
        assert_eq!(range.start.line, 2);
        assert_eq!(range.end.character, 27);
        assert_eq!(range.end.line, 2);
    }
}
