FROM public.ecr.aws/lambda/microvms:al2023-minimal

RUN dnf install -y \
        openssh-server \
        ca-certificates \
        git \
        wget \
        tar \
        gzip \
        unzip \
        which \
        less \
        vim \
        python3 \
        python3-pip \
        jq \
        tree \
        bash-completion \
        nodejs \
        npm && \
    ssh-keygen -A && \
    mkdir -p /run/sshd && \
    sed -ri 's/^#?PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config && \
    dnf clean all

RUN curl -fsSL https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip -o /tmp/awscli.zip && \
    unzip -q /tmp/awscli.zip -d /tmp && \
    /tmp/aws/install && \
    rm -rf /tmp/awscli.zip /tmp/aws

RUN npm install -g @anthropic-ai/claude-code

CMD ["sleep", "infinity"]
