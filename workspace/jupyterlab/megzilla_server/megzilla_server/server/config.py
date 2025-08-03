#!/usr/bin/env python
# -*- coding: utf-8 -*-
import os

aiservice_server = os.environ.get('AISERVICE_WORKSPACE_SERVER')
workspace_id = os.environ.get('AISERVICE_WORKSPACE')
instance_id = os.environ.get('AISERVICE_INSTANCE')

headers = {
    'X-Token': os.environ.get('AISERVICE_WORKSPACE_TOKEN'),
}