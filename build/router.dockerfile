FROM docker.io/library/golang:1.17-alpine AS build
WORKDIR /src

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o app -v cmd/router/main.go

FROM docker.io/library/alpine as final
WORKDIR /app

COPY --from=build /src/app ./app
COPY --from=build /src/static ./static

EXPOSE 8080 7676
ENTRYPOINT ["./app"]
