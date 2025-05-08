#!/bin/bash

USAGE="Usage: ./build.sh <Docker Hub Organization> <version>"

if [ "$1" == "--help" ] || [ "$#" -lt "2" ]; then
	echo $USAGE
	exit 0
fi

ORG=$1
VERSION=$2

rm -rf rootfs
docker plugin disable $ORG/docker-secretprovider-plugin:v$VERSION
docker plugin rm $ORG/docker-secretprovider-plugin:v$VERSION

docker plugin disable secret:latest
docker plugin rm secret:latest

mkdir -p rootfs
mkdir -p rootfs/etc/ssl/certs/
cp /etc/ssl/certs/ca-certificates.crt rootfs/etc/ssl/certs/
CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -extldflags "-static"'
cp docker-secretprovider-plugin rootfs/

docker plugin create $ORG/docker-secretprovider-plugin:v$VERSION .
docker plugin push $ORG/docker-secretprovider-plugin:v$VERSION

docker plugin rm $ORG/docker-secretprovider-plugin:v$VERSION
