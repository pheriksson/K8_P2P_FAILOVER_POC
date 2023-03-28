# synatx=docker/dockerfile:1

FROM golang:1.19-alpine

WORKDIR /app
COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./
COPY agent/* ./agent/

RUN go build -o /POC

EXPOSE 9999/udp

CMD ["/POC"]
