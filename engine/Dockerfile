FROM golang:1.27-alpine AS build

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /server .

# Alpine rather than an empty image: the Compose health check needs wget.
FROM alpine:3

COPY --from=build /server /server

USER nobody
EXPOSE 8080
ENTRYPOINT ["/server"]
