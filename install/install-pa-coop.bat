@echo off
rem pa-coop : double-clique sur ce fichier pour installer le multijoueur pa-coop de Prison Architect.
powershell -NoProfile -ExecutionPolicy Bypass -Command "[Net.ServicePointManager]::SecurityProtocol='Tls12'; iex ((New-Object Net.WebClient).DownloadString('https://raw.githubusercontent.com/fabienmillet/pa-coop/main/install/install.ps1'))"
if errorlevel 1 pause
