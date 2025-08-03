import typing

import refile
from snapdata.io import load_sds_iter


def convert_sds_to_imagenet(sds_paths: typing.Union[str, dict], output_dir: str, dsname: str = "train"):
    """
    将 sds 格式的文件转换为 imagenet (ILSVRC) 的格式，支持分类任务。
    sds_paths: sds 文件的路径，可以是单个文件，也可以是一个字典，key 为数据集的名称，value 为 sds 文件的路径。
    output_dir: 输出的文件夹根路径。
    dsname: 数据集的名称，如 "train"、"val" 等。
    """
    if isinstance(sds_paths, str):
        sds_paths = {"train": sds_paths}

    label_path = refile.smart_path_join(output_dir, "labels.txt")
    image_list_path = refile.smart_path_join(output_dir, f"{dsname}_list.txt")
    refile.smart_makedirs(output_dir, exist_ok=True)

    assert not refile.smart_exists(image_list_path), "image list already exists"
    if refile.smart_exists(label_path):
        legacy = True
        with refile.smart_open(label_path, "r") as f:
            categories = f.read().splitlines()
        category_name_to_category_id = {c: i for i, c in enumerate(categories)}
    else:
        legacy = False
        categories = []
        category_name_to_category_id = {}
    images = []

    for sds_path in sds_paths.values():
        # load sds
        sds_iter = load_sds_iter(sds_path)

        # convert sds to ImageNet format
        for line in sds_iter:
            image_id = len(images)
            image_filename = f"{dsname}_{image_id:08d}.jpg"
            class_names = [c for c in line["classes"] if c["type"] == "gt_cls"]
            assert len(class_names) == 1, "only one class per image is supported"
            class_name = class_names[0]["class_name"]
            class_name = class_name.split(".")[-1].replace(" ", "_")

            if legacy:
                assert class_name in category_name_to_category_id, "class name not found"
                category_id = category_name_to_category_id[class_name]
            else:
                if class_name not in category_name_to_category_id:
                    category_id = len(category_name_to_category_id)
                    category_name_to_category_id[class_name] = category_id
                    categories.append(class_name)
                else:
                    category_id = category_name_to_category_id[class_name]

            image_folder_path = refile.smart_path_join(output_dir, class_name)
            refile.smart_makedirs(image_folder_path, exist_ok=True)
            image_path = refile.smart_path_join(image_folder_path, image_filename)
            refile.smart_copy(line["url"], image_path)
            images.append(f"{class_name}/{image_filename} {category_id}")

    with refile.smart_open(image_list_path, "w") as f:
        f.write("\n".join(images))

    if not legacy:
        with refile.smart_open(label_path, "w") as f:
            f.write("\n".join(categories))


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument("sds", type=str, help="path to sds")
    parser.add_argument("output_dir", type=str, help="path to output")
    parser.add_argument("-d", "--dsname", type=str, help="folder name", default="train")
    args = parser.parse_args()

    convert_sds_to_imagenet(args.sds, args.output_dir, args.dsname)
