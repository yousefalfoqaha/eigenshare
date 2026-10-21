FROM golang:1.26.1-alpine AS build
WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download

COPY main.go .
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -o eigenshare .

FROM alpine:3.24.2
WORKDIR /app

RUN apk add --no-cache su-exec

COPY entrypoint.sh /app/entrypoint.sh
COPY --from=build /build/eigenshare /app/eigenshare

RUN chmod +x /app/entrypoint.sh

ENV SECURE_COOKIES=true
ENV DATA_PATH=/data
EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/eigenshare"]
