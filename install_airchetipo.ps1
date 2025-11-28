try {
    Invoke-WebRequest -Uri "https://github.com/opencode-ai/opencode/archive/refs/heads/main.zip" -OutFile "main.zip" -ErrorAction Stop
    Expand-Archive -Path main.zip -DestinationPath . -Force
    Remove-Item main.zip
} catch {
    Write-Host "Errore nel download: $_"
}
