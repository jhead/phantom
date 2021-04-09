@echo off
title Console Connector
color f0
set /p ip="Enter the server IP you wish to connect to"
set /p port="Enter the server PORT you wish to connect to"

./phantom-<os> -server lax.mcbr.cubed.host:19132
