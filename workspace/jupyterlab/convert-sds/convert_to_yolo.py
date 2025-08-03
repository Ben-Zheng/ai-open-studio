import json
import typing

import numpy as np
import refile
import yaml
from snapdata.io import load_sds_iter


def xyxy2xywhn1(x, w=640, h=640):
    # Convert nx4 boxes from [x1, y1, x2, y2] to [x, y, w, h] normalized where xy1=top-left, xy2=bottom-right
    y = np.copy(x)
    y[0] = ((x[0] + x[2]) / 2) / w  # x center
    y[1] = ((x[1] + x[3]) / 2) / h  # y center
    y[2] = (x[2] - x[0]) / w  # width
    y[3] = (x[3] - x[1]) / h  # height
    return y


def convert_sds_to_yolo(
    train: typing.Union[list, dict], val: typing.Union[list, dict], output_dir: str,dsname: str = "dataset"
):
    """
    将 sds 格式的文件转换为 yolo 的格式，支持检测任务。
    train: 训练集sds 文件的路径，可以是一个列表，也可以是一个字典，key 为数据集的名称，value 为 sds 文件的路径。
    val: 测试集sds 文件的路径，可以是一个列表，也可以是一个字典，key 为数据集的名称，value 为 sds 文件的路径。训练集和验证集必须同时提供。
    output_dir: 输出的文件夹根路径。
    使用样例：python3 convert_to_yolo.py --train s3://snapdet-benchmark/sds_data/6812c58214357abe44278fa3edb9f31c/data.sds --val s3://snapdet-benchmark/sds_data/20386da086b6cdd597b5ea10aa113782/data.sds -o /data/QA_test/testyolo
    """
    if isinstance(train, dict):
        train = list(train.values())
    if isinstance(val, dict):
        val = list(val.values())

    meta_path = refile.smart_path_join(output_dir, f"{dsname}.yaml")
    meta = {}
    meta["path"] = refile.smart_realpath(output_dir)
    meta["train"] = refile.smart_path_join("images", "train")  # (relative to 'path')
    meta["val"] = refile.smart_path_join("images", "val")  # (relative to 'path')
    meta["test"] = ""

    train_image_folder_path = refile.smart_path_join(output_dir, "images", "train")
    train_label_folder_path = refile.smart_path_join(output_dir, "labels", "train")
    val_image_folder_path = refile.smart_path_join(output_dir, "images", "val")
    val_label_folder_path = refile.smart_path_join(output_dir, "labels", "val")

    refile.smart_makedirs(output_dir, exist_ok=True)
    refile.smart_makedirs(train_image_folder_path, exist_ok=True)
    refile.smart_makedirs(train_label_folder_path, exist_ok=True)
    refile.smart_makedirs(val_image_folder_path, exist_ok=True)
    refile.smart_makedirs(val_label_folder_path, exist_ok=True)

    images = []
    category_name_to_category_id = {}

    for sds_path in train + val:
        # load sds
        sds_iter = load_sds_iter(sds_path)

        # convert sds to YOLO format
        for line in sds_iter:
            image_id = len(images)
            image_filename = f"{image_id:012d}.jpg"
            shape = (line["image_height"], line["image_width"])

            image = {
                "id": image_id,
                "width": shape[1],
                "height": shape[0],
                "file_name": image_filename,
                "license": 1,
            }
            images.append(image)
            if sds_path in train:
                image_path = refile.smart_path_join(
                    train_image_folder_path, image_filename
                )
            else:
                image_path = refile.smart_path_join(
                    val_image_folder_path, image_filename
                )
            labels, categories, raw_lb = [], [], []
            refile.smart_copy(line["url"], image_path)

            for b in line["boxes"]:
                if b["type"] == "detected_box" or b["type"] == "ignored_box":
                    continue

                class_name = b["class_name"]
                if class_name not in category_name_to_category_id:
                    category_id = len(category_name_to_category_id)
                    category_name_to_category_id[class_name] = category_id
                category_id = category_name_to_category_id[class_name]
                lb = xyxy2xywhn1(
                    np.array(
                        [
                            max(b["x"], 0),
                            max(b["y"], 0),
                            min(b["x"] + b["w"], shape[1]),
                            min(b["y"] + b["h"], shape[0]),
                        ],
                        dtype=np.float32,
                    ),
                    w=shape[1],
                    h=shape[0],
                )
                categories.append(category_id)
                labels.append(lb)
                raw_lb.append([category_id, *lb])
            if sds_path in train:
                label_path = refile.smart_path_join(
                    train_label_folder_path, image_filename.replace("jpg", "txt")
                )
            else:
                label_path = refile.smart_path_join(
                    val_label_folder_path, image_filename.replace("jpg", "txt")
                )
            with refile.smart_open(label_path, "w") as f:
                for class_lb in raw_lb:
                    f.write(" ".join([str(i) for i in class_lb]) + "\n")
    meta["names"] = {v: k for k, v in category_name_to_category_id.items()}
    with refile.smart_open(meta_path, "w") as f:
        yaml.safe_dump(meta, f)


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument(
        "-t",
        "--train",
        action="append",
        help="Training data, in SDS format. All training data will be combined together",
        required=True,
    )
    parser.add_argument(
        "-v",
        "--val",
        action="append",
        help="Validation data, in SDS format. All training data will be combined together",
        required=True,
    )
    parser.add_argument("-o", "--output_dir", type=str, help="path to output")
    parser.add_argument("-d", "--dsname", type=str, help="dataset name", default="dataset")
    args = parser.parse_args()

    convert_sds_to_yolo(args.train, args.val, args.output_dir, args.dsname)
