Workspace 常用操作手册
===

### 环境检查

```bash
pip show refile nori2 aissdk
```

确认环境中安装了 `refile>=7.0.6` `nori2==1.11.8` `aissdk>=2.0.6`

### nori / oss 读取

通过 refile 可以读取 nori 文件，refile 提供了 SDK 和 CLI 两套接口
workspace 中已经内置了 refile / nori2 等 nori 操作相关库

```bash
refile cp nori://50117312,a99000d587c0ca output.jpg
```

```python
from refile import smart_open, smart_load_content, smart_load_image

with smart_open('nori://50117312,a99000d587c0ca', 'rb') as fp:
    content = fp.read()
```

refile 是开源项目 megfile 的旷视内部使用版本，接口和 megfile 完全一致，额外加入了 nori 等旷视私有协议的支持
文档参考 https://megvii-research.github.io/megfile/readme.html

### AIS 相关操作

通过 aissdk 是 AIS 的一些常见操作的 SDK，workspace 中已经内置了 aissdk

```bash
# 配置 access_key / secret_key / tenant / project
# 其中 ACCESS_KEY / SECRET_KEY 可以在 https://train-ai.dlife.cn/personal/keys-management 中获得
#     TENANT / PROJECT 可以在 TODO 中获得
# 配置文件保存在 ~/.ais/config.yaml 中
ais config -a ACCESS_KEY -s SECRET_KEY -t TENANT -p PROJECT -e https://train-ai.dlife.cn

# 下载数据集（sds）
# 可以使用 AIS 前端页面的 URL、S3 路径、或 dataset_id + revision_id 下载数据集
ais datahub download https://train-ai.dlife.cn/ais/megvii/AIS-AIT/dataset/Project/641aa41a2fa595d02d433fff/641aa41a2fa595d02d434000
ais datahub download s3://system-megvii-face-dataset/dataset-641aa41a2fa595d02d433fff/revision-641aa41a2fa595d02d434000/entity.sds
ais datahub download 641aa41a2fa595d02d433fff 641aa41a2fa595d02d434000

# 上传数据集（sds）
# 注意：AIS 不支持上传同名数据集
ais datahub upload 我的数据集 entity.sds

# 下载模型
# 可以使用 AIS 前端页面的 URL、S3 路径、或 model_id + revision_id 下载数据集
ais modelhub download https://train-ai.dlife.cn/ais/megvii/AIS-AIT/model/detail/2nmf004wf2opfwsf8arvst43ssi/version/2nmezwmig8xygeug8rcs3ky798z
ais modelhub download s3://system-megvii-face-model/2nmf004wf2opfwsf8arvst43ssi/2nmezwmig8xygeug8rcs3ky798z/model.zip
ais modelhub download 2nmf004wf2opfwsf8arvst43ssi 2nmezwmig8xygeug8rcs3ky798z

# 上传模型
# 注意：AIS 同名模型会创建新的版本
ais modelhub upload -t Classification -p TRT -d 2080TI 我的模型 model.zip
```

```python
# 通过 SDK 完成上述操作
from ais.datahub import Client as DatahubClient
from ais.modelhub import Client as ModelhubClient

datahub = DatahubClient()
datahub.download_dataset(dataset_id, revision_id, output_path)
datahub.upload_dataset(
    dataset_name="我的数据集",
    ignore_labeling=False,
    file_path="entity.sds"
    show_process_bar=True,
)

modelhub = ModelhubClient()
modelhub.download_model(model_id, revision_id, output_path)
modelhub.upload_model(
    model_name="我的模型",
    model_desc="一些备注",
    app_type="Classification",  # Classification / Detection
    device="2080TI",
    platform="TRT",
    file_path="model.zip"
    show_process_bar=True,
)

```

### 附录

#### AISSDK 配置文件

```yaml
endpoint: https://train-ai.dlife.cn

# access_key, secret_key 可以在 AIS 前端看到
# 链接：https://train-ai.dlife.cn/personal/keys-management
access_key: c9f9e5e763493751ada7c9d23693ab61
secret_key: 6c4f9e8595564dc6671474aa10fc597c

# tenent / project 可以从 AIS 前端 URL 拿到
# 项目首页的 URL：https://train-ai.dlife.cn/ais/testqabdwwbh/project1/automaticLearning/
# 其中 testqabdwwbh / project1 分别是 tenent / project
# 现在需要抓包看 project，例如：chrome 的 F12，看请求头的 x-ais-tenant / x-ais-project
tenant: testqabdwwbh
project: testqafdoisx
```

#### 平台、设备支持列表

```yaml
TRT:
    - 2080TI
RKNN:
    - RV1109
    - RK3568
MAGIK:
    - T40
    - T41
CVITEK:
    - CV1835
    - CV1825
```
