FROM docker.io/golang:1.21 as builder

WORKDIR /build
COPY go.mod go.sum *.go ./
RUN go get -d .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o cursed-status-page .

FROM alpine:latest

WORKDIR /

COPY --from=builder /build/cursed-status-page ./
COPY static/. ./static/
COPY templates/. ./templates/
ENTRYPOINT ["./cursed-status-page"]
