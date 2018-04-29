FROM golang:1.8

WORKDIR /go/src/github.com/nov1n/kubernetes-heat-scheduling/monitor
COPY . .

RUN go build main.go

CMD ["main"]
