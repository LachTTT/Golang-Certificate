FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN adduser -D app
COPY --from=build /out/server /server
USER app
EXPOSE 8081
CMD ["/server"]
