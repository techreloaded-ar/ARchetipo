# ─── AIRchetipo Installer ─────────────────────────────────────────────────────
# Installs AIRchetipo skills + config for Claude Code, Codex, Gemini CLI, OpenCode, GitHub Copilot
# Usage: irm https://raw.githubusercontent.com/techreloaded-ar/AIRchetipo/main/install.ps1 | iex
#        .\install.ps1 [-Local] [-Cleanup] [-Help]
#   -Local    Installs from local .\skills\ folder instead of GitHub
#   -Cleanup  Removes installed skills from selected tools
#   -Help     Shows this help message
# ──────────────────────────────────────────────────────────────────────────────

param(
  [switch]$Local,
  [switch]$Cleanup,
  [switch]$Help
)

$ErrorActionPreference = "Stop"

$RepoZip    = "https://github.com/techreloaded-ar/AIRchetipo/archive/refs/heads/main.zip"
$SkillNames = @("airchetipo-autopilot", "airchetipo-design", "airchetipo-implement", "airchetipo-inception", "airchetipo-plan")

# ─── Tool definitions ─────────────────────────────────────────────────────────
$Tools = @(
  @{ Name = "Claude Code";     Path = ".claude\skills" }
  @{ Name = "Codex";           Path = ".agents\skills" }
  @{ Name = "Gemini CLI";      Path = ".gemini\skills" }
  @{ Name = "OpenCode";        Path = ".opencode\skills" }
  @{ Name = "GitHub Copilot";  Path = ".github\skills" }
)

# ─── Install for a specific tool ──────────────────────────────────────────────
function Install-ForTool {
  param([int]$ToolIndex, [string]$SourceDir)

  $tool     = $Tools[$ToolIndex]
  $toolPath = $tool.Path

  foreach ($skillName in $SkillNames) {
    $src = Join-Path $SourceDir $skillName
    if (-not (Test-Path $toolPath)) {
      New-Item -ItemType Directory -Path $toolPath -Force | Out-Null
    }
    Copy-Item -Path $src -Destination $toolPath -Recurse -Force
  }

  Write-Host ""
  Write-Host "  $([char]0x2713) " -ForegroundColor Green -NoNewline
  Write-Host "$($tool.Name)" -ForegroundColor White -NoNewline
  Write-Host " -> $toolPath" -ForegroundColor DarkGray
  foreach ($skillName in $SkillNames) {
    Write-Host "     $skillName/" -ForegroundColor DarkGray
  }
}

# ─── Cleanup for a specific tool ──────────────────────────────────────────────
function Remove-ForTool {
  param([int]$ToolIndex)

  $tool     = $Tools[$ToolIndex]
  $toolPath = $tool.Path
  $removed  = $false

  foreach ($skillName in $SkillNames) {
    $target = Join-Path $toolPath $skillName
    if (Test-Path $target) {
      Remove-Item -Path $target -Recurse -Force
      $removed = $true
    }
  }

  Write-Host ""
  if ($removed) {
    Write-Host "  $([char]0x2713) " -ForegroundColor Green -NoNewline
    Write-Host "$($tool.Name)" -ForegroundColor White -NoNewline
    Write-Host " -> rimosso da $toolPath" -ForegroundColor DarkGray
  } else {
    Write-Host "  - " -ForegroundColor Yellow -NoNewline
    Write-Host "$($tool.Name)" -ForegroundColor White -NoNewline
    Write-Host " -> nessuna skill trovata in $toolPath" -ForegroundColor DarkGray
  }
}

# ─── Interactive multi-select menu ────────────────────────────────────────────
function Show-Menu {
  $selected  = @(0) * $Tools.Count
  $cursor    = 0
  $toolCount = $Tools.Count

  # Check if we have an interactive console
  $isInteractive = $true
  try {
    [Console]::CursorVisible = $false
  } catch {
    $isInteractive = $false
  }

  if (-not $isInteractive) {
    return Show-FallbackMenu
  }

  try {
    # Initial draw
    for ($i = 0; $i -lt $toolCount; $i++) {
      $checkbox = if ($selected[$i] -eq 1) { "[x]" } else { "[ ]" }
      $prefix   = if ($i -eq $cursor) { ">" } else { " " }

      if ($i -eq $cursor) {
        Write-Host "  $prefix $checkbox $($Tools[$i].Name)" -ForegroundColor Cyan -NoNewline
      } else {
        Write-Host "  $prefix $checkbox $($Tools[$i].Name)" -NoNewline
      }
      Write-Host " ($($Tools[$i].Path))" -ForegroundColor DarkGray
    }
    Write-Host "  Up/Down: navigate  Space: toggle  Enter: confirm" -ForegroundColor DarkGray -NoNewline

    while ($true) {
      $key = [Console]::ReadKey($true)

      switch ($key.Key) {
        "UpArrow"   { if ($cursor -gt 0) { $cursor-- } }
        "DownArrow" { if ($cursor -lt ($toolCount - 1)) { $cursor++ } }
        "Spacebar"  { $selected[$cursor] = if ($selected[$cursor] -eq 1) { 0 } else { 1 } }
        "Enter" {
          Write-Host ""
          [Console]::CursorVisible = $true

          $result = @()
          for ($i = 0; $i -lt $toolCount; $i++) {
            if ($selected[$i] -eq 1) { $result += $i }
          }
          return $result
        }
      }

      # Redraw — move cursor up
      [Console]::SetCursorPosition(0, [Console]::CursorTop - $toolCount)

      for ($i = 0; $i -lt $toolCount; $i++) {
        $checkbox = if ($selected[$i] -eq 1) { "[x]" } else { "[ ]" }
        $prefix   = if ($i -eq $cursor) { ">" } else { " " }

        # Clear line
        Write-Host ("`r" + (" " * [Console]::WindowWidth)) -NoNewline
        Write-Host "`r" -NoNewline

        if ($i -eq $cursor) {
          Write-Host "  $prefix $checkbox $($Tools[$i].Name)" -ForegroundColor Cyan -NoNewline
        } else {
          Write-Host "  $prefix $checkbox $($Tools[$i].Name)" -NoNewline
        }
        Write-Host " ($($Tools[$i].Path))" -ForegroundColor DarkGray
      }
      Write-Host ("`r" + (" " * [Console]::WindowWidth)) -NoNewline
      Write-Host "`r  Up/Down: navigate  Space: toggle  Enter: confirm" -ForegroundColor DarkGray -NoNewline
    }
  } finally {
    try { [Console]::CursorVisible = $true } catch {}
  }
}

# ─── Fallback menu for non-interactive terminals ──────────────────────────────
function Show-FallbackMenu {
  Write-Host ""
  for ($i = 0; $i -lt $Tools.Count; $i++) {
    Write-Host "  $($i + 1)) $($Tools[$i].Name) ($($Tools[$i].Path))"
  }
  Write-Host ""
  $choices = Read-Host "Enter tool numbers separated by spaces (e.g. 1 2 3), or 'all'"

  $result = @()
  if ($choices -eq "all") {
    for ($i = 0; $i -lt $Tools.Count; $i++) { $result += $i }
  } elseif (-not [string]::IsNullOrWhiteSpace($choices)) {
    foreach ($c in $choices -split '\s+') {
      try {
        $idx = [int]$c - 1
        if ($idx -ge 0 -and $idx -lt $Tools.Count) {
          $result += $idx
        }
      } catch {
        # Ignore invalid input
      }
    }
  }
  return $result
}

# ─── Backend selection (radio-button, single choice) ─────────────────────────
$BackendOptions      = @("file", "github")
$BackendDescriptions = @("backlog e planning come file Markdown locali", "backlog e planning su GitHub Projects v2 — richiede GitHub CLI")

function Show-BackendMenu {
  $cursor = 0
  $optCount = $BackendOptions.Count

  $isInteractive = $true
  try {
    [Console]::CursorVisible = $false
  } catch {
    $isInteractive = $false
  }

  if (-not $isInteractive) {
    return Show-FallbackBackend
  }

  try {
    # Initial draw
    for ($i = 0; $i -lt $optCount; $i++) {
      $radio  = if ($i -eq $cursor) { "(x)" } else { "( )" }
      $prefix = if ($i -eq $cursor) { ">" } else { " " }

      if ($i -eq $cursor) {
        Write-Host "  $prefix $radio $($BackendOptions[$i])" -ForegroundColor Cyan -NoNewline
      } else {
        Write-Host "  $prefix $radio $($BackendOptions[$i])" -NoNewline
      }
      Write-Host "  $($BackendDescriptions[$i])" -ForegroundColor DarkGray
    }
    Write-Host "  Up/Down: navigate  Enter: confirm" -ForegroundColor DarkGray -NoNewline

    while ($true) {
      $key = [Console]::ReadKey($true)

      switch ($key.Key) {
        "UpArrow"   { if ($cursor -gt 0) { $cursor-- } }
        "DownArrow" { if ($cursor -lt ($optCount - 1)) { $cursor++ } }
        "Enter" {
          Write-Host ""
          [Console]::CursorVisible = $true
          return $BackendOptions[$cursor]
        }
      }

      # Redraw
      [Console]::SetCursorPosition(0, [Console]::CursorTop - $optCount)

      for ($i = 0; $i -lt $optCount; $i++) {
        $radio  = if ($i -eq $cursor) { "(x)" } else { "( )" }
        $prefix = if ($i -eq $cursor) { ">" } else { " " }

        Write-Host ("`r" + (" " * [Console]::WindowWidth)) -NoNewline
        Write-Host "`r" -NoNewline

        if ($i -eq $cursor) {
          Write-Host "  $prefix $radio $($BackendOptions[$i])" -ForegroundColor Cyan -NoNewline
        } else {
          Write-Host "  $prefix $radio $($BackendOptions[$i])" -NoNewline
        }
        Write-Host "  $($BackendDescriptions[$i])" -ForegroundColor DarkGray
      }
      Write-Host ("`r" + (" " * [Console]::WindowWidth)) -NoNewline
      Write-Host "`r  Up/Down: navigate  Enter: confirm" -ForegroundColor DarkGray -NoNewline
    }
  } finally {
    try { [Console]::CursorVisible = $true } catch {}
  }
}

function Show-FallbackBackend {
  Write-Host ""
  for ($i = 0; $i -lt $BackendOptions.Count; $i++) {
    Write-Host "  $($i + 1)) $($BackendOptions[$i])  ($($BackendDescriptions[$i]))"
  }
  Write-Host ""
  $choice = Read-Host "Select backend [1]"

  if ($choice -eq "2") {
    return "github"
  }
  return "file"
}

# ─── Install config ──────────────────────────────────────────────────────────
function Install-Config {
  param([string]$SourceDir, [string]$Backend)

  $configDir  = ".airchetipo"
  $configFile = Join-Path $configDir "config.yaml"

  # Determine source config path
  $sourceConfig = Join-Path (Split-Path $SourceDir -Parent) "config.yaml"
  if (-not (Test-Path $sourceConfig)) {
    Write-Host ""
    Write-Host "  - " -ForegroundColor Yellow -NoNewline
    Write-Host "config.yaml non trovato nella source, skip" -ForegroundColor DarkGray
    return
  }

  # Check if config already exists
  if (Test-Path $configFile) {
    Write-Host ""
    Write-Host "  ! " -ForegroundColor Yellow -NoNewline
    Write-Host ".airchetipo\config.yaml esiste gia. Sovrascrivere? [s/N] " -NoNewline
    $answer = Read-Host
    if ($answer -ne "s" -and $answer -ne "S" -and $answer -ne "y" -and $answer -ne "Y") {
      Write-Host "  Config non modificato" -ForegroundColor DarkGray
      return
    }
  }

  if (-not (Test-Path $configDir)) {
    New-Item -ItemType Directory -Path $configDir -Force | Out-Null
  }
  Copy-Item -Path $sourceConfig -Destination $configFile -Force

  # Update backend value
  $content = Get-Content $configFile -Raw
  $content = $content -replace "^backend:.*", "backend: $Backend"
  Set-Content -Path $configFile -Value $content -NoNewline

  Write-Host ""
  Write-Host "  $([char]0x2713) " -ForegroundColor Green -NoNewline
  Write-Host ".airchetipo\config.yaml" -ForegroundColor White -NoNewline
  Write-Host " (backend: $Backend)" -ForegroundColor DarkGray
}

# ─── Main ─────────────────────────────────────────────────────────────────────
function Main {
  Write-Host ""
  if ($Help) {
    Write-Host @"

AIRchetipo Installer

Usage:
  irm https://raw.githubusercontent.com/techreloaded-ar/AIRchetipo/main/install.ps1 | iex
  .\install.ps1 [-Local] [-Cleanup] [-Help]

Flags:
  -Local    Install from local .\skills\ folder instead of downloading from GitHub
  -Cleanup  Remove installed skills from selected tools
  -Help     Show this help message

Skills installed:
  airchetipo-autopilot
  airchetipo-design
  airchetipo-implement
  airchetipo-inception
  airchetipo-plan

Configuration:
  .airchetipo\config.yaml is created with the selected backend (file or github).

Supported tools:
  Claude Code, Codex, Gemini CLI, OpenCode, GitHub Copilot
"@
    return
  }

  Write-Host "  AIRchetipo Installer" -ForegroundColor Cyan

  if ($Cleanup) {
    Write-Host "  Remove AIRchetipo skills from your tools" -ForegroundColor DarkGray
    Write-Host ""
    Write-Host ""

    # Tool selection
    Write-Host "  Select tools to clean up:" -ForegroundColor White
    Write-Host ""
    $selectedTools = Show-Menu

    if ($null -eq $selectedTools -or $selectedTools.Count -eq 0) {
      Write-Host "  No tools selected. Exiting." -ForegroundColor Yellow
      return
    }

    # Cleanup
    Write-Host "  Cleaning up..." -ForegroundColor White

    foreach ($toolIndex in $selectedTools) {
      Remove-ForTool -ToolIndex $toolIndex
    }

    # Summary
    Write-Host ""
    Write-Host "  Done! " -ForegroundColor Green -NoNewline
    Write-Host "Cleaned up $($selectedTools.Count) tool(s)."
    Write-Host ""
    return
  }

  Write-Host "  Install AIRchetipo skills for your tools" -ForegroundColor DarkGray
  Write-Host ""

  $sourceDir = ""
  $tempDir   = $null

  if ($Local) {
    # Use local .\skills\ folder
    if (-not (Test-Path ".\skills")) {
      Write-Host "  Error: .\skills\ folder not found. Run from the repository root." -ForegroundColor Red
      return
    }
    $sourceDir = ".\skills"
    Write-Host "  Using local skills folder..." -ForegroundColor DarkGray
  } else {
    # Create temp directory
    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "airchetipo-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    Write-Host "  Downloading skills..." -ForegroundColor DarkGray
    $zipFile = Join-Path $tempDir "airchetipo.zip"

    try {
      Invoke-WebRequest -Uri $RepoZip -OutFile $zipFile -UseBasicParsing -ErrorAction Stop
    } catch {
      Write-Host "  X Failed to download repository" -ForegroundColor Red
      if ($null -ne $tempDir -and (Test-Path $tempDir)) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
      }
      return
    }

    Expand-Archive -Path $zipFile -DestinationPath $tempDir -Force
    $sourceDir = Join-Path $tempDir "AIRchetipo-main\skills"

    Write-Host "  $([char]0x2713) Downloaded skills" -ForegroundColor Green
  }

  Write-Host ""

  # Tool selection
  Write-Host "  Select tools to install for:" -ForegroundColor White
  Write-Host ""
  $selectedTools = Show-Menu

  if ($null -eq $selectedTools -or $selectedTools.Count -eq 0) {
    Write-Host "  No tools selected. Exiting." -ForegroundColor Yellow
    if ($null -ne $tempDir -and (Test-Path $tempDir)) {
      Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    return
  }

  # Backend selection
  Write-Host "  Select backend:" -ForegroundColor White
  Write-Host ""
  $selectedBackend = Show-BackendMenu

  # Install
  Write-Host "  Installing..." -ForegroundColor White

  foreach ($toolIndex in $selectedTools) {
    Install-ForTool -ToolIndex $toolIndex -SourceDir $sourceDir
  }

  # Install config
  Install-Config -SourceDir $sourceDir -Backend $selectedBackend

  # Summary
  Write-Host ""
  Write-Host "  Done! " -ForegroundColor Green -NoNewline
  Write-Host "Installed $($SkillNames.Count) skill(s) for $($selectedTools.Count) tool(s)."
  Write-Host ""

  # Cleanup temp directory
  if ($null -ne $tempDir -and (Test-Path $tempDir)) {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
  }
}

Main
