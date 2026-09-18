#!/bin/sh
# Called by staredit after every change, even a failed one.
set -e
systemctl start starrupture
