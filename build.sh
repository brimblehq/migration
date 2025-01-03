GOOS=linux GOARCH=amd64 go build -o executables/setup cmd/setup/main.go

GOOS=darwin GOARCH=amd64 go build -o executables/setup-mac cmd/setup/main.go