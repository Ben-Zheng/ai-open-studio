package client

var template = `
{
    "apiVersion": "workspace.aiservice.brainpp.cn/v1",
    "kind": "AWorkspace",
    "metadata": {
        "namespace": "dev",
        "name": "workspace-sample"
    },
    "spec": {
        "envs": [
            {
                "name": "AISERVICE_WORKSPACE",
                "value": "84"
            },
            {
                "name": "AISERVICE_INSTANCE",
                "value": "94"
            },
            {
                "name": "AISERVICE_BASE_URL",
                "value": "/lab/workspaces/84"
            },
            {
                "name": "AISERVICE_SERVER",
                "value": "https://aishhb.brainpp.cn"
            },
            {
                "name": "AISERVICE_TOKEN",
                "value": "kuuedyii-8d5b0820-3a74-4b19-a6f9-399e103651c1"
            }
        ],
        "image": "docker-registry-internal.i.brainpp.cn/brain/ais-workspace:dev",
        "instanceID": "",
        "labels": {
            "app": "workspace-84",
            "college": "5782",
            "instance": "94",
            "workspace": "84"
        },
        "ports": [
            {
                "name": "lab",
                "port": 80,
                "protocol": "TCP",
                "targetPort": 80
            },
            {
                "name": "vscode",
                "port": 9826,
                "protocol": "TCP",
                "targetPort": 9826
            }
        ],
        "resources": {
            "limits": {
                "cpu": "2",
                "memory": "8Gi"
            }
        },
        "size": 1,
        "workspaceID": ""
    }
}
`
