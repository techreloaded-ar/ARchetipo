# Builds the packaged skill bundles (.zip) for Copilot Studio.
#
#   .\build-skills.ps1            builds archetipo-design.zip and archetipo-analysis.zip
#   .\build-skills.ps1 -List      also lists the entries of each archive
#
# Each archive has SKILL.md at its root and references/, scripts/ and assets/ next to it, with no
# extra top-level directory, as "Upload a skill" expects. The archive is written inside the skill
# folder and is not tracked by git. Requires PowerShell 7.

[CmdletBinding()]
param([switch]$List)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem

$skillsRoot = Join-Path $PSScriptRoot 'skills'
$staging = Join-Path ([System.IO.Path]::GetTempPath()) ("archetipo-skills-" + [guid]::NewGuid().ToString('N'))
$bundled = @('archetipo-design', 'archetipo-analysis')
$parts = @('SKILL.md', 'references', 'scripts', 'assets')

try {
    foreach ($skill in $bundled) {
        $src = Join-Path $skillsRoot $skill
        if (-not (Test-Path (Join-Path $src 'SKILL.md'))) { throw "SKILL.md not found in $src" }
        $dst = Join-Path $staging $skill
        New-Item -ItemType Directory -Force $dst | Out-Null
        foreach ($part in $parts) {
            $p = Join-Path $src $part
            if (Test-Path $p) { Copy-Item $p (Join-Path $dst $part) -Recurse }
        }
        Get-ChildItem $dst -Recurse -Include '__pycache__', '*.pyc' -Force | Remove-Item -Recurse -Force
        $zip = Join-Path $src "$skill.zip"
        if (Test-Path $zip) { Remove-Item $zip -Force }
        [System.IO.Compression.ZipFile]::CreateFromDirectory($dst, $zip, [System.IO.Compression.CompressionLevel]::Optimal, $false)
        $size = [math]::Round((Get-Item $zip).Length / 1KB)
        Write-Host "built $zip ($size KB)"
        if ($List) {
            $a = [System.IO.Compression.ZipFile]::OpenRead($zip)
            try { $a.Entries | ForEach-Object { Write-Host ("  {0,-48} {1,8}" -f $_.FullName, $_.Length) } }
            finally { $a.Dispose() }
        }
    }
}
finally {
    if (Test-Path $staging) { Remove-Item $staging -Recurse -Force }
}
