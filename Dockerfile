FROM golang:1.26.1-alpine AS build
WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download

COPY main.go .
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -o eigenshare .

FROM alpine:3.24.2
WORKDIR /app

COPY --from=build /build/eigenshare /app/eigenshare

EXPOSE 8080
ENV SECURE_COOKIES=true

CMD ["/app/eigenshare"]
