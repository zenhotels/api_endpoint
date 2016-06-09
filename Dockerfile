FROM golang:1.6

WORKDIR /go/src/apps.hotcore.in/api_endpoint
ADD . /go/src/apps.hotcore.in/api_endpoint
RUN GOBIN=/usr/bin && \
go build -o /usr/bin/api_endpoint apps.hotcore.in/api_endpoint && \
rm -rf /go/src/apps.hotcore.in/api_endpoint

ENTRYPOINT /usr/bin/api_endpoint
EXPOSE 8080
EXPOSE 10000
