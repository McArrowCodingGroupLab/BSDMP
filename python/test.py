from bsdmp import BSDMPPack, BSDMPUnpack, BSDMPFieldType, BSDMPFieldSize

msg = BSDMPPack(compression=0)
msg.set_flag(1, True)

msg.title(
    [
        ("username", BSDMPFieldType.STRING, BSDMPFieldSize.K64),
        ("age", BSDMPFieldType.INT, BSDMPFieldSize.B255),
        ("active", BSDMPFieldType.BOOL, BSDMPFieldSize.B255),
        ("name", BSDMPFieldType.STRING, BSDMPFieldSize.K64),
        ("friends", BSDMPFieldType.JSON, BSDMPFieldSize.G4),
    ]
)

msg.frame(
    {
        "username": "Alice",
        "age": 30,
        "active": True,
        "name": "Alice Mitnick",
        "friends": ["alex"],
    }
)
msg.frame(
    {
        "username": "boB",
        "active": False,
        "friends": ["carol", "david", "eve", "fred", "greg"],
    }
)
msg.frame(
    {
        "username": "caRol",
        "age": 40,
        "active": True,
        "name": "Carol Jenkins",
        "friends": [
            "david",
            "eve",
            "fred",
            "greg",
            "harry",
            "ian",
            "jane",
            "kate",
            "lily",
            "mary",
        ],
    }
)

bytes_message = msg.pack()

print("Размер сообщения:", len(bytes_message))
print("HEX:", bytes_message.hex())


res = BSDMPUnpack(bytes_message, True)
for frame in res.frames:
    print(frame)
print(res.get_flag(1))
