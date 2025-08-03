import json
import typing

import refile
from snapdata.io import load_sds_iter


def convert_sds_to_coco(sds_paths: typing.Union[str, dict], output_dir: str, dsname: str = "train2017"):
    """
    将 sds 格式的文件转换为 imagenet (ILSVRC) 的格式，支持检测任务。
    sds_paths: sds 文件的路径，可以是单个文件，也可以是一个字典，key 为数据集的名称，value 为 sds 文件的路径。
    output_dir: 输出的文件夹根路径。
    dsname: 数据集的名称，如 "train2017"、"val2017" 等。
    """
    if isinstance(sds_paths, str):
        sds_paths = {"train": sds_paths}

    image_folder_path = refile.smart_path_join(output_dir, dsname)
    annotation_path = refile.smart_path_join(output_dir, "annotations")

    refile.smart_makedirs(output_dir, exist_ok=True)
    refile.smart_makedirs(image_folder_path, exist_ok=True)
    refile.smart_makedirs(annotation_path, exist_ok=True)

    images, annotations, categories = [], [], []
    category_name_to_category_id = {}

    for sds_path in sds_paths.values():
        # load sds
        sds_iter = load_sds_iter(sds_path)

        # convert sds to COCO format
        for line in sds_iter:
            image_id = len(images)
            image_filename = f"{image_id:08d}.jpg"
            image_height, image_width = line["image_height"], line["image_width"]

            image = {
                "id": image_id,
                "width": image_width,
                "height": image_height,
                "file_name": image_filename,
                "license": 1,
                "flickr_url": "",
                "coco_url": "",
                "date_captured": "",
            }
            images.append(image)

            image_path = refile.smart_path_join(image_folder_path, image_filename)
            refile.smart_copy(line["url"], image_path)

            for box in line["boxes"]:
                if box["type"] == "detected_box":
                    continue
                elif box["type"] == "gt_box":
                    iscrowd = 0
                else:
                    iscrowd = 1

                class_name = box["class_name"]
                if class_name not in category_name_to_category_id:
                    category_id = len(category_name_to_category_id)
                    category_name_to_category_id[class_name] = category_id
                    category = {
                        "id": category_id,
                        "name": class_name,
                        "supercategory": class_name,
                    }
                    categories.append(category)
                category_id = category_name_to_category_id[class_name]

                annotation = {
                    "id": len(annotations),
                    "image_id": image_id,
                    "category_id": category_id,
                    "iscrowd": iscrowd,
                    "area": box["w"] * box["h"],
                    "bbox": [box["x"], box["y"], box["w"], box["h"]],
                    "segmentation": [],
                }
                annotations.append(annotation)

    info = {
        "description": "snapdata",
        "url": "",
        "version": "1.0",
        "year": 1970,
        "contributor": "",
        "date_created": "",
    }

    license = []

    annot = {
        "info": info,
        "images": images,
        "annotations": annotations,
        "categories": categories,
        "licenses": license,
    }

    annot_path = refile.smart_path_join(annotation_path, f"instances_{dsname}.json")
    with refile.smart_open(annot_path, "w") as f:
        json.dump(annot, f)


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument("sds", type=str, help="path to sds")
    parser.add_argument("output_dir", type=str, help="path to output")
    parser.add_argument("-d", "--dsname", type=str, help="folder name", default="train2017")
    args = parser.parse_args()

    convert_sds_to_coco(args.sds, args.output_dir, args.dsname)
