FROM ubuntu:latest

RUN apt-get update && apt-get install -y software-properties-common
RUN apt-get update && apt-get install -y git openssh-server tmux vim fish python-dev python-pip python3-dev python3-pip ca-certificates curl wget

RUN add-apt-repository -y ppa:neovim-ppa/stable
RUN apt-get update && apt-get install -y neovim

RUN wget https://dl.google.com/go/go1.13.1.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go1.13.1.linux-amd64.tar.gz
ENV GOPATH=/root/go
ENV PATH=$PATH:/usr/local/go/bin:$GOPATH/bin


COPY vimrc /root/.vimrc
COPY vimrc /root/.config/nvim/init.vim
COPY tmux.conf /root/.tmux.conf

RUN ssh-keygen -A

RUN pip3 install --user pynvim
RUN curl -fLo ~/.local/share/nvim/site/autoload/plug.vim --create-dirs \
    https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
RUN curl -fLo ~/.vim/autoload/plug.vim --create-dirs \
    https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
RUN vim +PlugInstall +qall
RUN nvim +PlugInstall +UpdateRemotePlugins +qall
# RUN nvim +GoInstallBinaries +qall
RUN echo "colorscheme molokai" >> /root/.vimrc

RUN go get github.com/klauspost/asmfmt/cmd/asmfmt
RUN go get github.com/go-delve/delve/cmd/dlv
RUN go get github.com/kisielk/errcheck
RUN go get github.com/davidrjenni/reftools/cmd/fillstruct
RUN go get github.com/mdempsky/gocode
RUN go get github.com/stamblerre/gocode
RUN go get github.com/rogpeppe/godef
RUN go get github.com/zmb3/gogetdoc
RUN go get golang.org/x/tools/cmd/goimports
RUN go get golang.org/x/lint/golint
RUN go get golang.org/x/tools/cmd/gopls
RUN go get github.com/alecthomas/gometalinter
RUN go get github.com/golangci/golangci-lint/cmd/golangci-lint
RUN go get github.com/fatih/gomodifytags
RUN go get golang.org/x/tools/cmd/gorename
RUN go get github.com/jstemmer/gotags
RUN go get golang.org/x/tools/cmd/guru
RUN go get github.com/josharian/impl
RUN go get honnef.co/go/tools/cmd/keyify
RUN go get github.com/fatih/motion
RUN go get github.com/koron/iferr

RUN go get github.com/onsi/ginkgo/ginkgo
RUN go get github.com/golang/mock/mockgen

WORKDIR /root/dev/src/gitlab.com/akita
RUN git clone https://gitlab.com/akita/akita.git
RUN git clone https://gitlab.com/akita/util.git
RUN git clone https://gitlab.com/akita/mem.git
RUN git clone https://gitlab.com/akita/noc.git
RUN git clone https://gitlab.com/akita/gcn3.git


COPY sshd_config /etc/ssh/sshd_config

mkdir -p ~/.config/fish/
touch ~/.config/fish/config.fish
RUN echo "set -gx GOPATH /root/go" >> ~/.config/fish/config.fish 
RUN echo "set -gx PATH /usr/local/go/bin \$GOPATH/bin \$PATH" >> ~/.config/fish/config.fish

EXPOSE 22
ENTRYPOINT ["/bin/entry_point.sh"]
