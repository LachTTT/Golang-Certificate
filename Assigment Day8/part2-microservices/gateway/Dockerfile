FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/gateway .

FROM alpine:3.20
RUN adduser -D app
COPY --from=build /out/gateway /gateway
USER app
EXPOSE 8080
CMD ["/gateway"]
