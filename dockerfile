# Base image:
FROM golang:1.11

# Install golint
ENV GOPATH /go
ENV PATH ${GOPATH}/bin:$PATH
RUN go get -u github.com/golang/lint/golint

#CMD ["/bin/bash"]
#WORKDIR /go