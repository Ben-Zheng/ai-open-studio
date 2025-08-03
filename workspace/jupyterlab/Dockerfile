FROM docker-registry.i.brainpp.cn/nvidia/cuda:10.1-base-ubuntu18.04
# FROM nvidia/cuda:11.4.0-cudnn8-devel-ubuntu18.04

RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys A4B469963BF863CC && apt-get update

ENV LIBRARY_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ \
    DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

# Packages
RUN sed -i -e "s/archive.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g" /etc/apt/sources.list && \
    sed -i -e "s/security.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g" /etc/apt/sources.list && \
    apt-get update && apt-get install ca-certificates tzdata -y && \
    apt-get update && apt-get install zip -y

# 分多层可以避免特定包下载问题导致的整层垮掉的问题
RUN apt-get update && apt-get install --assume-yes --fix-missing apt-transport-https locales man less gzip rsync tmux && rm -rf /var/lib/apt/lists/*
RUN apt-get update && apt-get install --assume-yes --fix-missing ibglib2.0-0 libsm6 libxext6 libxrender-dev pciutils python python3 python3-distutils && rm -rf /var/lib/apt/lists/*
RUN apt-get update && apt-get install --assume-yes --fix-missing sudo curl vim wget unzip openssl ca-certificates nginx build-essential libgl1-mesa-glx && rm -rf /var/lib/apt/lists/*
RUN apt-get update && apt-get install --assume-yes --fix-missing gdb python3-dbg ffmpeg cmake openssh-server && rm -rf /var/lib/apt/lists/*

RUN ln -sf /usr/share/zoneinfo/Asia/Hong_Kong /etc/localtime
RUN dpkg-reconfigure -f noninteractive tzdata

# 安装 docker client
RUN apt-get update && apt-get install -y systemd
RUN apt install apt-transport-https ca-certificates curl gnupg-agent software-properties-common -y
RUN curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
RUN add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
RUN apt-get update -y
RUN apt install docker-ce docker-ce-cli containerd.io -y

RUN mkdir /usr/local/aiservice
# sshd 配置
# 方案一， 直接放置文件
# RUN mkdir /root/.ssh
# COPY ssh/authorized_keys /root/.ssh/authorized_keys
# RUN chmod 755 /root/.ssh && chmod 644 /root/.ssh/authorized_keys

# 方案二，更改 sshd_config 文件
RUN mkdir /root/.ssh && chmod 755 /root/.ssh
COPY ssh/sshd_config /etc/ssh/sshd_config
COPY ssh/authorized_keys /etc/ssh/authorized_keys
COPY ssh/authorized_keys.sh /etc/ssh/authorized_keys.sh
RUN chmod a+x /etc/ssh/authorized_keys.sh

COPY ssh/env /usr/local/aiservice/env
RUN cat /usr/local/aiservice/env >> /root/.bashrc
COPY ssh/profile /root/.profile

# Config pip
COPY pip.conf /etc/pip.conf

RUN curl -o /tmp/get-pip.py -O http://deploy.i.brainpp.cn/megstudio/get-pip.py
RUN python3 /tmp/get-pip.py && rm /tmp/get-pip.py

COPY jupyter3 /etc/jupyter

WORKDIR /home/aiservice
COPY work.ipynb work.ipynb

# Download anaconda install
ENV PATH /home/aiservice/.miniconda/bin:$PATH
RUN curl -O http://deploy.i.brainpp.cn/megstudio/Miniconda3-latest-Linux-x86_64.sh && chmod +x Miniconda3-latest-Linux-x86_64.sh && bash Miniconda3-latest-Linux-x86_64.sh -b -p .miniconda && rm Miniconda3-latest-Linux-x86_64.sh

# Add Anaconda to the Path
RUN conda create -c conda-forge -n ais --strict-channel-priority 'python >=3.7,<3.8' 'jupyterlab>=3.0.0,<4.0.0a0' jupyterlab-lsp jupyter-lsp-python python-lsp-server cookiecutter git

SHELL ["conda", "run", "-n", "ais", "/bin/bash", "-c"]

RUN pip install ipympl nvidia-ml-py3 jupyterlab-git tensorflow tensorboard jupyterlab-tensorboard-pro
RUN pip install nbresuse psutil ipympl pycocotools filterpy
RUN pip install awscli boto3 loguru protobuf opencv-python megfile numpy albumentations matplotlib
RUN pip install 'refile>=7.0.6' 'nori2==1.11.8' 'aissdk>=2.0.6' 'snapdata==0.0.8' -i http://mirrors.i.brainpp.cn/pypi/simple/ --extra-index-url http://pypi.i.brainpp.cn/brain/dev/+simple --trusted-host mirrors.i.brainpp.cn --trusted-host pypi.i.brainpp.cn
COPY README.md README.md
COPY convert-sds convert-sds

ENV LD_LIBRARY_PATH /usr/lib/x86_64-linux-gnu/:$LD_LIBRARY_PATH
ENV PYTHONPATH /home/aiservice/workspace/.py/lib/python3.7/site-packages
ENV PATH /home/aiservice/workspace/.py/bin:$PATH
ADD pip.conf /home/aiservice/.pip/pip.conf
RUN echo "prefix=/home/aiservice/workspace/.py" >> /home/aiservice/.pip/pip.conf

ENV PATH ~/.local/bin:$PATH

# 配置 vscode
RUN wget https://github.com/coder/code-server/releases/download/v4.9.1/code-server-4.9.1-linux-amd64.tar.gz && tar xvfz code-server-4.9.1-linux-amd64.tar.gz -C /usr/local && mv /usr/local/code-server-4.9.1-linux-amd64 /usr/local/vscode && rm code-server-4.9.1-linux-amd64.tar.gz

# 初始化脚本
COPY init.py /usr/local/aiservice/init.py

COPY resource.py /usr/local/aiservice/resource.py

ENTRYPOINT [ "/bin/sh", "-c", "service ssh start;conda run -n ais python /usr/local/aiservice/init.py; ( /usr/local/vscode/bin/code-server --bind-addr=0.0.0.0:9826 --auth=none --disable-telemetry --user-data-dir=/opt/vscode/ & ); conda run -n ais jupyter lab --ip=0.0.0.0 --config=/etc/jupyter/jupyter_notebook_config.py --allow-root" ]
