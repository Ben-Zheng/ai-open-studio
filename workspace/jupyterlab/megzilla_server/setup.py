#!/usr/bin/env python3
from setuptools import setup
from distutils.util import convert_path

main_ns = {}
ver_path = convert_path('megzilla_server/server/version.py')
with open(ver_path) as ver_file:
    exec(ver_file.read(), main_ns)

setup(
    name='megzilla_server',
    version = main_ns['version'],
    description = 'Jupyter extensions',
    author = 'aiservice',
    author_email = 'aiservice@megvii.com',
    install_requires = [
        'notebook >= 5.0.0',
        'tornado >= 6.0.2',
        'requests >= 2.23.0'
    ],
    packages = ['megzilla_server'],
    include_package_data = True,
)



