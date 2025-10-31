#!/bin/bash -eu
go build
msg='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
content_length(){
  printf '%s' "$msg"|wc -c|xargs
}
response_have="$({ printf 'Content-Length: %d\r\n\r\n%s' "$(content_length)" "${msg}" ;}|./make-language-server)"
response_want(){
printf 'Content-Length: 102\r\n\r\n{"id":1,"result":{"capabilities":{"completionProvider":{},"definitionProvider":true}},"jsonrpc":"2.0"}'
}
test "$response_have" = "$(response_want)" \
&& echo "${0} success" \
|| {
  echo "${0} failure"
  printf '\nresponse_have\n---\n%s\n\n' "$response_have"
  echo '==='
  printf '\nresponse_want\n---\n%s\n' "$(response_want)"
}
