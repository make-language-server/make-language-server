#!/bin/bash -eu
go build
content_length() {
  printf '%s' "$1" | wc -c | xargs
}
message_client_initialize='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
response_have="$({ printf 'Content-Length: %d\r\n\r\n%s' "$(content_length "$message_client_initialize")" "$message_client_initialize" ;} | ./make-language-server)"
response_want() {
  content='{"id":1,"result":{"capabilities":{"completionProvider":{},"definitionProvider":true,"textDocumentSync":1}},"jsonrpc":"2.0"}'
  printf 'Content-Length: %d\r\n\r\n%s' "$(content_length "$content")" "$content"
}
test "$response_have" = "$(response_want)" \
&& echo "${0} success" \
|| {
  echo "${0} failure"
  printf '\nresponse_have\n---\n%s\n\n' "$response_have"
  echo '==='
  printf '\nresponse_want\n---\n%s\n' "$(response_want)"
}
