def _jupyter_server_extension_paths():
    return [
        dict(module = "megzilla_server.server")
    ]

def load_jupyter_server_extension(nbapp):
    nbapp.log.info("megzilla server init")

