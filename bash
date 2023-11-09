#!/bin/ash

rm /.dockerenv

if [ -z "$1" ]; then
    /bin/ash -i
else
    /bin/ash -c "$@"
fi