import typing

import refile
import xmltodict
from snapdata.io import load_sds_iter


def convert_sds_to_voc(sds_paths: typing.Union[str, dict], output_dir: str, dsname: str = "train"):
    """
    将 sds 格式的文件转换为 pascal voc 的格式，同时支持分类和检测的任务。
    sds_paths: sds 文件的路径，可以是单个文件，也可以是一个字典，key 为数据集的名称，value 为 sds 文件的路径。
    output_dir: 输出的文件夹根路径。
    dsname: 数据集的名称，如 "train"、"val" 等。
    """
    if isinstance(sds_paths, str):
        sds_paths = {"train": sds_paths}

    image_folder_path = refile.smart_path_join(output_dir, "JPEGImages")
    annotation_path = refile.smart_path_join(output_dir, "Annotations")
    imageset_path = refile.smart_path_join(output_dir, "ImageSets", "Main")

    refile.smart_makedirs(output_dir, exist_ok=True)
    refile.smart_makedirs(image_folder_path, exist_ok=True)
    refile.smart_makedirs(annotation_path, exist_ok=True)
    refile.smart_makedirs(imageset_path, exist_ok=True)

    imageset = []
    for sds_path in sds_paths.values():
        # load sds
        sds_iter = load_sds_iter(sds_path)

        # convert sds to VOC format
        for line in sds_iter:
            image_id = len(imageset)
            image_filename = f"{dsname}_{image_id:08d}.jpg"
            imageset.append(f"{dsname}_{image_id:08d}")
            image_height, image_width = line["image_height"], line["image_width"]
            image_path = refile.smart_path_join(image_folder_path, image_filename)
            refile.smart_copy(line["url"], image_path)

            objects = []
            if "boxes" in line:
                for box in line["boxes"]:
                    if box["type"] != "gt_box":
                        continue
                    obj = {
                        "name": box["class_name"],
                        "bndbox": {
                            "xmin": box["x"],
                            "ymin": box["y"],
                            "xmax": box["x"] + box["w"],
                            "ymax": box["y"] + box["h"],
                        },
                    }
                    objects.append(obj)
            else:
                class_names = [c for c in line["classes"] if c["type"] == "gt_cls"]
                assert len(class_names) == 1, "only one class per image is supported"
                class_name = class_names[0]["class_name"]
                class_name = class_name.split(".")[-1].replace(" ", "_")
                objects.append({"name": class_name})

            annot = {
                "annotation": {
                    "filename": image_filename,
                    "path": refile.smart_path_join(image_folder_path, image_filename),
                    "source": {"database": "Unknown"},
                    "size": {"width": image_width, "height": image_height, "depth": 3},
                    "segmented": 0,
                    "object": objects,
                }
            }

            xml_path = refile.smart_path_join(annotation_path, f"{dsname}_{image_id:08d}.xml")
            xmltodict.unparse(annot, open(xml_path, "w"), pretty=True)

    # write imageset
    with open(refile.smart_path_join(imageset_path, f"{dsname}.txt"), "w") as f:
        f.write("\n".join(imageset))


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument("sds", type=str, help="path to sds")
    parser.add_argument("output_dir", type=str, help="path to output")
    parser.add_argument("-d", "--dsname", type=str, help="folder name", default="train")
    args = parser.parse_args()

    convert_sds_to_voc(args.sds, args.output_dir, args.dsname)
