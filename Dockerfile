FROM golang:1.10-alpine
RUN apk update
RUN apk add openssl ca-certificates git
RUN mkdir -p /go/src/influx-api
ADD server.go  /go/src/influx-api/server.go
ADD structs /go/src/influx-api/structs
ADD create.sql /go/src/influx-api/create.sql
ADD build.sh /build.sh
RUN chmod +x /build.sh
RUN /build.sh
WORKDIR /go/src/influx-api/
CMD ["./server"]
EXPOSE 3000


