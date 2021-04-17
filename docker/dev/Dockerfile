FROM golang:latest

RUN apt-get update && apt-get install -y software-properties-common
RUN apt-get update && apt-get install -y git openssh-server ca-certificates curl wget

RUN ssh-keygen -A

RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

RUN go get github.com/onsi/ginkgo/ginkgo
RUN go get github.com/golang/mock/mockgen

WORKDIR /root/dev/src/gitlab.com/akita
RUN git clone https://gitlab.com/akita/akita.git
RUN git clone https://gitlab.com/akita/util.git
RUN git clone https://gitlab.com/akita/mem.git
RUN git clone https://gitlab.com/akita/noc.git
RUN git clone https://gitlab.com/akita/mgpusim.git


COPY sshd_config /etc/ssh/sshd_config

EXPOSE 22
ENTRYPOINT ["/bin/entry_point.sh"]
