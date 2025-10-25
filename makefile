.PHONY: \
	install \
	test \

test: \
		main.go \
		main_test.go \

	test "$$(make -s -f testdata/captureCompletions.mk)" = 'echo_target head_tail' \
	&& go test

install:
	go install
