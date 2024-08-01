FROM golang:latest AS build
ADD . /root/src
WORKDIR /root/src
RUN CGO_ENABLED=0 go build -o /root/pancake ./cmd/proxy

FROM alpine/openssl AS certs
WORKDIR /root
RUN openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -sha256 -days 3650 -nodes -subj "/CN=Pancake default certificate"

FROM scratch
COPY --from=build /root/pancake /root/pancake
COPY --from=certs /root/server.crt /etc/pancake/server.crt
COPY --from=certs /root/server.key /etc/pancake/server.key
WORKDIR /root
ENTRYPOINT [ "/root/pancake" ]
EXPOSE 8080
