@echo off
title Console Connector
color f0

echo Enter the server IP you wish to connect to
set /p ip=

echo Enter the server PORT you wish to connect to
set /p port=

echo Attempting to Lauch Connection...
./phantom-Windows -server %ip%:%port%
echo success!
timeout /t 3 >nul
