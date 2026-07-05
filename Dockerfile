FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
COPY --from=build /bin/server /bin/server
EXPOSE 8090
ENTRYPOINT ["/bin/server"]
