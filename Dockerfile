FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/pulsecheck ./cmd/pulsecheck

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -g 65532 -S pulsecheck \
    && adduser -S -D -H -u 65532 -G pulsecheck pulsecheck \
    && mkdir -p /data \
    && chown pulsecheck:pulsecheck /data
COPY --from=build /out/pulsecheck /usr/local/bin/pulsecheck
USER pulsecheck
ENV PORT=8080 \
    SQLITE_PATH=/data/pulsecheck.db
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/pulsecheck"]
