#!/bin/sh
# Called by staredit before every change. Must only return once the server has
# saved and fully exited. Adapt to however you run your server.
set -e
# systemctl stop blocks until the unit's ExecStop (graceful save + exit) is done.
systemctl stop starrupture
