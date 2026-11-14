& {
	$ErrorActionPreference = 'Stop'
	$ProgressPreference = 'SilentlyContinue'
	[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

	$base = 'https://github.com/ahmd-sh/cloche/releases/latest/download'

	$cpu = try { [string][Runtime.InteropServices.RuntimeInformation]::OSArchitecture } catch { $env:PROCESSOR_ARCHITECTURE }
	$arch = switch ($cpu) {
		{ $_ -in 'X64', 'AMD64' } { 'amd64'; break }
		{ $_ -in 'Arm64' } { 'arm64'; break }
		default { throw "There's no build for $cpu. Build it from source: go install github.com/ahmd-sh/cloche/cmd/cloche@latest" }
	}

	$dir = if ($env:CLOCHE_INSTALL_DIR) { $env:CLOCHE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\cloche' }
	$name = "cloche-windows-$arch.zip"
	$tmp = Join-Path ([IO.Path]::GetTempPath()) ('cloche-' + [Guid]::NewGuid())
	New-Item -ItemType Directory $tmp | Out-Null
	try {
		Write-Host "Downloading $name"
		Invoke-WebRequest -UseBasicParsing "$base/$name" -OutFile (Join-Path $tmp $name)
		Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
		$want = Get-Content (Join-Path $tmp 'checksums.txt') | ForEach-Object {
			$f = $_.Trim() -split '\s+'
			if ($f.Count -eq 2 -and $f[1] -eq $name) { $f[0] }
		} | Select-Object -First 1
		if (-not $want) { throw "checksums.txt doesn't list $name" }
		$got = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $name)).Hash
		if ($got -ne $want) { throw "$name doesn't match its checksum" }
		Expand-Archive (Join-Path $tmp $name) -DestinationPath (Join-Path $tmp 'x')

		New-Item -ItemType Directory -Force $dir | Out-Null
		$exe = Join-Path $dir 'cloche.exe'
		if (Test-Path $exe) {
			Remove-Item -Force "$exe.old" -ErrorAction SilentlyContinue
			Move-Item -Force $exe "$exe.old"
		}
		Move-Item (Join-Path $tmp 'x\cloche.exe') $exe
	} finally {
		Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
	}
	Write-Host "Installed $(& $exe version) in $dir"

	$path = [Environment]::GetEnvironmentVariable('Path', 'User')
	if (-not (($path -split ';') -contains $dir)) {
		[Environment]::SetEnvironmentVariable('Path', ((@($path -split ';' | Where-Object { $_ }) + $dir) -join ';'), 'User')
		Write-Host "Added $dir to your PATH. New terminals will have it."
	}
	if (-not (($env:Path -split ';') -contains $dir)) { $env:Path = "$env:Path;$dir" }

	Write-Host ''
	& $exe setup
}
