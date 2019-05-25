FROM ubuntu:latest

COPY vimrc /root/.vimrc
COPY vimrc /root/.config/nvim/init.vim
COPY tmux.conf /root/.tmux.conf

RUN apt-get update
RUN apt-get install -y software-properties-common
RUN apt-get install -y git openssh-server tmux vim zsh python-dev python-pip python3-dev python3-pip ca-certificates curl wget

RUN add-apt-repository -y ppa:neovim-ppa/stable
RUN apt-get update
RUN apt-get install -y neovim

RUN wget https://dl.google.com/go/go1.12.5.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go1.12.5.linux-amd64.tar.gz
ENV GOPATH=/root/go
ENV PATH=$PATH:/usr/local/go/bin:$GOPATH/bin

RUN ssh-keygen -A
RUN sh -c "$(curl -fsSL https://raw.githubusercontent.com/robbyrussell/oh-my-zsh/master/tools/install.sh)"
RUN pip3 install --user pynvim

RUN curl -fLo ~/.local/share/nvim/site/autoload/plug.vim --create-dirs \
    https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
RUN curl -fLo ~/.vim/autoload/plug.vim --create-dirs \
    https://raw.githubusercontent.com/junegunn/vim-plug/master/plug.vim
RUN vim +PlugInstall +qall
RUN nvim +PlugInstall +UpdateRemotePlugins +qall
# RUN nvim +GoInstallBinaries +qall
RUN echo "colorscheme molokai" >> /root/.vimrc

RUN go get -u github.com/onsi/ginkgo/ginkgo
RUN go get -u github.com/golang/mock/mockgen

WORKDIR /root/dev/src/gitlab.com/akita
RUN git clone https://gitlab.com/akita/akita.git
RUN git clone https://gitlab.com/akita/util.git
RUN git clone https://gitlab.com/akita/mem.git
RUN git clone https://gitlab.com/akita/noc.git
RUN git clone https://gitlab.com/akita/gcn3.git

EXPOSE 22
ENTRYPOINT [ "tmux" ]