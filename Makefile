.PHONY: build run
build:
	GOOS=js GOARCH=wasm go build -o ./static/ds-addr-gen.wasm main.go

run: build
	goexec 'http.ListenAndServe(`:8080`, http.FileServer(http.Dir(`./static`)))'