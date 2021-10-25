FROM golang:1.17 AS build
WORKDIR /go/src

COPY go.mod .
COPY go.sum .
RUN go get -d -v ./...

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o router -v cmd/router/main.go

FROM ubuntu:21.10 as final
WORKDIR /go/app

COPY --from=build /go/src/router ./router
COPY --from=build /go/src/static ./static

EXPOSE 8080 7676
CMD ["/go/app/router"]
