FROM rockylinux:8.5 AS base

# Add ca certs so we can build the image on vpn
COPY cas.pem /etc/pki/ca-trust/source/anchors/custom_cas.pem 
RUN update-ca-trust

RUN set -eux -o pipefail && \
    dnf install -y nss-tools openssl ca-certificates && \
    dnf install -y curl dnf-utils dnf-plugins-core epel-release findutils git git-lfs iproute jq ncurses rsync sudo tree which zsh && \
    dnf config-manager --set-enabled powertools && \ 
    dnf install -y cmake gcc gcc-c++ glib2-devel opensc pcsc-lite-devel softhsm procps-ng protobuf-compiler protobuf-devel rpm-build 

FROM base AS user

ARG REPO_NAME=goxel
ARG USERNAME=gopher
ARG USER_UID=1000
ARG USER_GID=1000

# Create the user
RUN groupadd --gid ${USER_GID} ${USERNAME} && \
  useradd --uid ${USER_UID} --gid ${USER_GID} -m ${USERNAME} -s /bin/zsh && \
  echo ${USERNAME} ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/${USERNAME} && \
  chmod 0440 /etc/sudoers.d/${USERNAME}

########### NON ROOT FROM HERE DOWN ###########
USER ${USERNAME}

# envs
ENV HOME="/home/${USERNAME}"
ENV SDK_HOME=${HOME}/sdk
ENV PATH=${HOME}/bin:${PATH}

RUN set -eux && \
    mkdir -p ${HOME}/bin && \
    mkdir -p ${HOME}/go/bin && \
    sudo mkdir -p /workdir/${REPO_NAME} && \
    sudo chown ${USERNAME}:${USERNAME} /workdir

# pretty bash
COPY --chown=${USERNAME}:${USERNAME} .bash_pretty ${HOME}/.bash_pretty
RUN set -eux && \
    echo "source ${HOME}/.bash_pretty" >> ${HOME}/.bashrc

# go - (update with 'go install golang.org/dl/go1.25.1@latest && go1.25.1 download')
ENV GOPRIVATE=""
ENV PATH=${SDK_HOME}/go/bin:${PATH}
RUN set -eux && \
    export VERSION="1.25.1" && \
    export ARCH="$(arch | sed 's/aarch64/arm64/' | sed 's/x86_64/amd64/')" && \
    cd /tmp && mkdir -p ${SDK_HOME} && \
    curl -k -o go.tar.gz -L https://go.dev/dl/go${VERSION}.linux-${ARCH}.tar.gz && \
    tar xzf go.tar.gz && mv go ${SDK_HOME} && rm /tmp/go.tar.gz && \
    cd ${SDK_HOME} && mv go go${VERSION} && ln -s go${VERSION} go

# agentic ai
ENV NVM_DIR="${HOME}/.nvm"
RUN mkdir -p ${NVM_DIR} && \
    curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash && \
    source "$NVM_DIR/nvm.sh" && \
    nvm install 22 && \
    echo "source $NVM_DIR/nvm.sh" >> "$HOME/.bash_npm" && \
    echo "source $NVM_DIR/bash_completion" >> "$HOME/.bash_npm" && \
    echo "nvm install 22" >> "$HOME/.bash_npm" && \
    echo "source ${HOME}/.bash_npm" >> ${HOME}/.bashrc && \
    npm install -g @google/gemini-cli && \
    npm install -g @anthropic-ai/claude-code