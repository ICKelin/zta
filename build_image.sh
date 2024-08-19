./build.sh

cp release/zta-gw_linux_amd64 ./docker-build/zta

cd docker-build
docker build -t ickelin/zta .
docker push ickelin/zta:latest