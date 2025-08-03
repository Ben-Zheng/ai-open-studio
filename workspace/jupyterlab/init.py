import os
import requests

from loguru import logger


def get_notebook_url():
    domain = os.getenv("DOMAIN")
    ui_config_path = os.getenv("UI_CONFIG_PATH")
    url = f"https://{domain}{ui_config_path}"
    r = requests.Response()
    sampleNotebooks = []
    samplePictures = []
    try:
        r = requests.get(url=url, verify=False)
        sampleNotebooks = r.json().get("data").get(
            "dataAugmentation").get("sampleNotebooks")
        samplePictures = r.json().get("data").get(
            "dataAugmentation").get("samplePictures")
    except Exception as e:
        logger.error(e)
        raise e
    return sampleNotebooks, samplePictures


def download_notebook(key: str, url_path: str):
    dir_path = os.getenv("SAMPLE_DIR_PATH")
    try:
        os.makedirs(dir_path, exist_ok=True)
    except Exception as e:
        logger.error(e)
        raise e

    file_name = key + ".ipynb"
    file_path = os.path.join(dir_path, file_name)

    domain = os.getenv("DOMAIN")
    url = f"https://{domain}{url_path}"
    r = requests.get(url=url, verify=False)
    with open(file_path, 'wb') as f:
        f.write(r.content)


def download_notebook_picture(url_path: str):
    dir_path = os.getenv("SAMPLE_DIR_PATH")
    try:
        os.makedirs(dir_path, exist_ok=True)
    except Exception as e:
        logger.error(e)
        raise e

    file_name = url_path.split("/")[-1]
    file_path = os.path.join(dir_path, file_name)

    domain = os.getenv("DOMAIN")
    url = f"https://{domain}{url_path}"
    r = requests.get(url=url, verify=False)
    with open(file_path, 'wb') as f:
        f.write(r.content)


def create_docker_config():
    harbor_endpoint = os.getenv("HARBOR_ENDPOINT")
    harbor_token = os.getenv("HARBOR_TOKEN")
    dir_path = os.path.expanduser("~/.docker")
    try:
        os.makedirs(dir_path, exist_ok=True)
    except Exception as e:
        logger.error(e)
        raise e

    file_path = os.path.join(dir_path, 'config.json')
    config = '''{ "auths": { "%s": {"auth": "%s"} }}''' % (
        harbor_endpoint, harbor_token)

    with open(file_path, 'w') as f:
        f.write(config)


def replace_file_content(file_path: str, replace_content: dict):
    with open(file_path, "r", encoding="utf-8") as f:
        content = f.read()
        for source in replace_content:
            content = content.replace(source, replace_content[source])

    with open(file_path, "w", encoding="utf-8") as f:
        f.write(content)


def change_download_source():
    pypi_dct = {}
    pypi_index_url = os.getenv("PYPI_INDEX_URL")
    if pypi_index_url:
        source = "index-url = https://pypi.tuna.tsinghua.edu.cn/simple/"
        target = f"index-url = {pypi_index_url}"
        pypi_dct[source] = target

    pypi_trusted_host = os.getenv("PYPI_TRUSTED_HOST")
    if pypi_trusted_host:
        source = "trusted-host = pypi.tuna.tsinghua.edu.cn"
        target = f"trusted-host = {pypi_trusted_host}"
        pypi_dct[source] = target

    if len(pypi_dct) > 0:
        replace_file_content("/etc/pip.conf", pypi_dct)

    ubuntu_dct = {}
    ubuntu_index_url = os.getenv("UBUNTU_INDEX_URL")
    if ubuntu_index_url:
        source = "http://mirrors.tuna.tsinghua.edu.cn/ubuntu/"
        ubuntu_dct[source] = ubuntu_index_url
    if len(ubuntu_dct) > 0:
        replace_file_content("/etc/apt/sources.list", ubuntu_dct)


if __name__ == "__main__":
    # 创建 harbor 的配置文件
    try:
        create_docker_config()
    except Exception as e:
        logger.error(e)

    # 下载数据增强的相关文件
    sampleNotebooks, samplePictures = get_notebook_url()
    for sampleNotebook in sampleNotebooks:
        try:
            download_notebook(sampleNotebook["key"], sampleNotebook["url"])
        except Exception as e:
            logger.error(e)
    for samplePicture in samplePictures:
        try:
            download_notebook_picture(samplePicture)
        except Exception as e:
            logger.error(e)

    # 修改 Ubuntu 和 pypi 下载源
    try:
        change_download_source()
    except Exception as e:
        logger.error(e)
