FROM docker.io/library/golang:1.17 AS build
WORKDIR /go/src

COPY go.mod .
COPY go.sum .
RUN go get -d -v ./...

COPY . .
RUN go build -o router -v cmd/router/main.go

FROM docker.io/library/fedora:35 as final
WORKDIR /go/app

COPY --from=build /go/src/router ./router
COPY --from=build /go/src/static ./static

EXPOSE 8080 7676
ENTRYPOINT ["/go/app/router"]
