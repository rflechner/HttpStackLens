$goroot = go env GOROOT
mkdir -ErrorAction Ignore webui\wwwroot\js
Copy-Item -Force "$goroot\lib\wasm\wasm_exec.js" -Destination "webui\wwwroot\js\wasm_exec.js"

# Compile Go to WASM
$env:GOOS = "js"
$env:GOARCH = "wasm"
go build -o webui\wwwroot\wasm\app.wasm .\webui\wasm\

# back to normal
$env:GOOS = ""
$env:GOARCH = ""
