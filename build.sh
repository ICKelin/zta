rm -r release
GOOS=linux GOARCH=amd64 go build -o release/zta-gw_linux_amd64 gateway/*.go
GOOS=linux GOARCH=amd64 go build -o release/zta-client_linux_amd64 client/*.go

GOOS=darwin GOARCH=amd64 go build -o release/zta-gw_darwin_amd64 gateway/*.go
GOOS=darwin GOARCH=amd64 go build -o release/zta-client_darwin_amd64 client/*.go

cp etc/* release/
