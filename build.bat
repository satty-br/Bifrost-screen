@echo off
REM Compila o bifrost.exe (precisa do Go 1.24+ instalado: https://go.dev/dl/)
setlocal
cd /d "%~dp0"
set VERSION=1.0.0
if not "%~1"=="" set VERSION=%~1

echo Gerando icone e manifesto...
go run ./tools/genres %VERSION% || goto :erro

echo Compilando bifrost.exe %VERSION%...
set CGO_ENABLED=0
go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=%VERSION%" -o dist\bifrost.exe .\cmd\bifrost || goto :erro

echo.
echo Pronto: dist\bifrost.exe
exit /b 0

:erro
echo Falhou.
exit /b 1
