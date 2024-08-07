FROM --platform=$BUILDPLATFORM golang:latest AS build
ARG TARGETOS
ARG TARGETARCH
ADD . /root/src
WORKDIR /root/src
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /root/pancake ./cmd/proxy

FROM --platform=$BUILDPLATFORM alpine/openssl AS certs
WORKDIR /root
RUN openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -sha256 -days 3650 -nodes -subj "/CN=Pancake default certificate"
RUN touch config.yaml

FROM scratch
COPY --from=build /root/pancake /root/pancake
COPY --from=certs /root/server.crt /etc/pancake/server.crt
COPY --from=certs /root/server.key /etc/pancake/server.key
COPY --from=certs /root/config.yaml /etc/pancake/config.yaml
WORKDIR /root
ENTRYPOINT [ "/root/pancake" ]
EXPOSE 8080
