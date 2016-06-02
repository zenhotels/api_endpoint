FROM golang

WORKDIR /go/src/apps.hotcore.in/api_endpoint
ADD . /go/src/apps.hotcore.in/api_endpoint
ENV GOBIN=/usr/bin
RUN go build -o /usr/bin/api_endpoint apps.hotcore.in/api_endpoint

ENTRYPOINT /usr/bin/api_endpoint
EXPOSE 8080
