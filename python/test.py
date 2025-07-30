import json
from bsdmp import BSDMPClient, BSDMPServer, CompressionType

# КЛИЕНТСКАЯ ЧАСТЬ
msg = BSDMPClient(compression=CompressionType.GZIP)
msg.format(["name", "age", "active"])
msg.frame(["Alice", 25, True])
msg.frame(["Bob", 30, False])
encoded_bytes = msg.encode()
print("Encoded hex:", encoded_bytes.hex())

# СЕРВЕРНАЯ ЧАСТЬ
encoded_bytes = bytes.fromhex(
    "0100000001000000780000001f8b08000000000000ff348c410a023110046bb249fc81fa0c51bc8910416f5efcc118460944058dbe5f24eea569e8ae7240040ea774dc6f27c0a6d729e0b9ebcd6440af2611cdad7c4cfa2ec01c08a45ab239966b4f7bbeedcff7fce967c0c0ee7176ac16818bd6d778fa060000ffff97d8e9e87e000000"
)
server = BSDMPServer()
server.decode(encoded_bytes)
for i, frame in enumerate(server.frames):
    print(f"=== Frame {i + 1} ===")
    for key, value in frame.items():
        if isinstance(value, bytes):
            print(f"{key}=0x{value.hex()}")
        elif isinstance(value, (list, dict)):
            print(f"{key}={json.dumps(value)}")
        else:
            print(f"{key}={value}")
    print()
