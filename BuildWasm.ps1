$goroot = go env GOROOT
mkdir -ErrorAction Ignore webui\wwwroot\js
Copy-Item -Force "$goroot\lib\wasm\wasm_exec.js" -Destination "webui\wwwroot\js\wasm_exec.js"

# Compile Go to WASM - navigate to the wasm directory first
Push-Location webui\wasm
$env:GOOS = "js"
$env:GOARCH = "wasm"
go build -o ..\wwwroot\wasm\app.wasm .

# back to normal
$env:GOOS = ""
$env:GOARCH = ""
Pop-Location