# travel-tickets-api

```sh
docker login
docker build --platform linux/amd64 -t yuriydubinin100/travel-tickets-api:1.0.0 .
docker push yuriydubinin100/travel-tickets-api:1.0.0
```

```sh
docker pull yuriydubinin100/travel-tickets-api:1.0.0

docker run -d \
  --name travel-tickets-api \
  --env-file .env \
  --add-host=host.docker.internal:host-gateway \
  -p 18080:8080 \
  --user root \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /run/systemd:/run/systemd:ro \
  -v /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro \
  -v travel-tickets-ssh:/data/ssh \
  -v /usr/libexec/docker/cli-plugins/docker-compose:/usr/libexec/docker/cli-plugins/docker-compose:ro \
  yuriydubinin100/travel-tickets-api:1.0.0
```