@echo off
setlocal
cd /d "%~dp0"
chcp 65001 >nul 2>nul
title livetool - Wails dev

rem ---------------------------------------------------------------- locate Go
rem Go is not always on the user PATH, so probe the usual install dirs.
where go >nul 2>nul
if errorlevel 1 (
  if exist "C:\Program Files\Go\bin\go.exe" set "PATH=C:\Program Files\Go\bin;%PATH%"
)
where go >nul 2>nul
if errorlevel 1 (
  if exist "%LOCALAPPDATA%\Programs\Go\bin\go.exe" set "PATH=%LOCALAPPDATA%\Programs\Go\bin;%PATH%"
)
where go >nul 2>nul
if errorlevel 1 (
  if exist "C:\Go\bin\go.exe" set "PATH=C:\Go\bin;%PATH%"
)
where go >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go toolchain not found.
  echo         Install it with:  winget install GoLang.Go
  echo.
  pause
  exit /b 1
)

rem -------------------------------------------------------------- locate Node
rem Node is not always on the user PATH (fnm/nvm shims only inject into the
rem shell that ran "fnm use"), so probe the usual install dirs like Go above.
where node >nul 2>nul
if errorlevel 1 if exist "C:\Program Files\nodejs\node.exe" set "PATH=C:\Program Files\nodejs;%PATH%"
where node >nul 2>nul
if errorlevel 1 if exist "%ProgramFiles(x86)%\nodejs\node.exe" set "PATH=%ProgramFiles(x86)%\nodejs;%PATH%"
where node >nul 2>nul
if errorlevel 1 if exist "%LOCALAPPDATA%\Programs\nodejs\node.exe" set "PATH=%LOCALAPPDATA%\Programs\nodejs;%PATH%"
where node >nul 2>nul
if errorlevel 1 if exist "%APPDATA%\fnm\aliases\default\node.exe" set "PATH=%APPDATA%\fnm\aliases\default;%PATH%"
where node >nul 2>nul
if errorlevel 1 if exist "%APPDATA%\nvm\node.exe" set "PATH=%APPDATA%\nvm;%PATH%"
where node >nul 2>nul
if errorlevel 1 for /d %%D in ("%APPDATA%\fnm\node-versions\v*") do if exist "%%D\installation\node.exe" set "PATH=%%D\installation;%PATH%"
where node >nul 2>nul
if errorlevel 1 for /d %%D in ("%LOCALAPPDATA%\fnm\node-versions\v*") do if exist "%%D\installation\node.exe" set "PATH=%%D\installation;%PATH%"
where node >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Node.js not found. Node.js 20 or newer is required.
  echo         Not on PATH and none of the usual install dirs matched.
  echo         fnm users: run "fnm use 24" in this folder first, or
  echo         install Node.js with:  winget install OpenJS.NodeJS.LTS
  echo.
  pause
  exit /b 1
)
for /f "delims=" %%V in ('node -v') do echo       Node.js %%V

echo [1/4] Generating Wails bindings ...
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.25 generate bindings -ts -i -d frontend/bindings
if errorlevel 1 goto fail

echo [2/4] Checking frontend dependencies ...
if not exist "frontend\node_modules\.package-lock.json" (
  echo       node_modules missing, running npm ci ...
  call npm --prefix frontend ci
  if errorlevel 1 goto fail
) else (
  echo       node_modules present, skipping.
)

echo [3/4] Checking frontend build output ...
if not exist "frontend\dist\index.html" (
  echo       frontend/dist missing, running vite build ...
  call npm --prefix frontend run build
  if errorlevel 1 goto fail
) else (
  echo       frontend/dist present, skipping.
)

echo [4/4] Starting Wails dev mode ...
echo       Vite dev server : http://127.0.0.1:9245
echo       Desktop window  : AKA直播
echo       Close the app window or press Ctrl+C to stop.
echo.
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.25 dev --config ./build/config.yml --port 9245
set "CODE=%ERRORLEVEL%"

echo.
if not "%CODE%"=="0" (
  echo [ERROR] Wails dev exited with code %CODE%.
) else (
  echo Wails dev stopped.
)
echo.
pause
exit /b %CODE%

:fail
echo.
echo [ERROR] The command above failed. See its output for details.
echo.
pause
exit /b 1
