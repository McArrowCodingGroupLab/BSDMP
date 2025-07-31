```
▼ Header (24 bytes)
00000000  03010000                              |....            | Version = 3
00000004  00000000 00000000                     |........        | Compression = NONE (0)
0000000c  00000000 00000000                     |........        | BinaryFlags = 0
00000014  4d010000                              |M...            | DataSize = 333 bytes
00000018  00000000 00000000                     |........        | Reserved2 = 0

▼ Data Block (333 bytes)
00000020  03000000                              |....            | FrameCount = 3
00000024  03000000                              |....            | FSE_Length = 3
00000028  fafbfc                                |...             | FSE = 0xFAFBFC
0000002b  03000000                              |....            | FEE_Length = 3
0000002f  fdfeff                                |...             | FEE = 0xFDFEFF
00000032  30000000                              |0...            | TitleLength = 48

▼ Title (48 bytes)
00000036  0800                                  |..              | NameLen = 8
00000038  757365726e616d65                      |username        | Name = "username"
00000040  0102                                  |..              | Type = INT (1), TypeLen = 2
00000042  0300                                  |..              | NameLen = 3
00000044  616765                                |age             | Name = "age"
00000047  0201                                  |..              | Type = STRING (2), TypeLen = 1
00000049  0600                                  |..              | NameLen = 6
0000004b  616374697665                          |active          | Name = "active"
00000051  0401                                  |..              | Type = BOOL (4), TypeLen = 1
00000053  0400                                  |..              | NameLen = 4
00000055  6e616d65                              |name            | Name = "name"
00000059  0102                                  |..              | Type = INT (1), TypeLen = 2
0000005b  0700                                  |..              | NameLen = 7
0000005d  667269656e6473                        |friends         | Name = "friends"
00000064  0504                                  |..              | Type = ARRAY (5), TypeLen = 4

▼ Frame 1
00000066  fafbfc                                |...             | FSE = 0xFAFBFC
00000069  01000000                              |....            | FrameNum = 1
0000006d  0500                                  |..              | FieldCount = 5
0000006f  00                                    |.               | TitleIndex = 0 (username)
00000070  0500                                  |..              | FieldLen = 5
00000072  416c696365                            |Alice           | Data = "Alice"
00000077  01                                    |.               | TitleIndex = 1 (age)
00000078  08                                    |.               | FieldLen = 8
00000079  1e00000000000000                      |........        | Data = 30
00000081  02                                    |.               | TitleIndex = 2 (active)
00000082  01                                    |.               | FieldLen = 1
00000083  01                                    |.               | Data = True
00000084  03                                    |.               | TitleIndex = 3 (name)
00000085  0d00                                  |..              | FieldLen = 13
00000087  416c696365204d69746e69636b            |Alice Mitnick   | Data = "Alice Mitnick"
00000094  04                                    |.               | TitleIndex = 4 (friends)
00000095  08000000                              |....            | FieldLen = 8
00000099  5b22616c6578225d                      |["alex"]        | Data = ["alex"]
000000a1  fdfeff                                |...             | FEE = 0xFDFEFF

▼ Frame 2
000000a4  fafbfc                                |...             | FSE = 0xFAFBFC
000000a7  02000000                              |....            | FrameNum = 2
000000ab  0300                                  |..              | FieldCount = 3
000000ad  00                                    |.               | TitleIndex = 0 (username)
000000ae  0300                                  |..              | FieldLen = 3
000000b0  626f42                                |boB             | Data = "boB"
000000b3  01                                    |.               | TitleIndex = 1 (age)
000000b4  01                                    |.               | FieldLen = 1
000000b5  00                                    |.               | Data = 0
000000b6  04                                    |.               | TitleIndex = 4 (friends)
000000b7  29000000                              |)...            | FieldLen = 41
000000bb  5b226361726f6c222c20226461766964222c  |["carol", "david",
000000d5  2022657665222c202266726564222c202267  | "eve", "fred", "g
000000ef  726567225d                            |reg"]           | Data = ["carol","david","eve","fred","greg"]
000000f2  fdfeff                                |...             | FEE = 0xFDFEFF

▼ Frame 3
000000f5  fafbfc                                |...             | FSE = 0xFAFBFC
000000f8  03000000                              |....            | FrameNum = 3
000000fc  0500                                  |..              | FieldCount = 5
000000fe  00                                    |.               | TitleIndex = 0 (username)
000000ff  0500                                  |..              | FieldLen = 5
00000101  6361526f6c                            |caRol           | Data = "caRol"
00000106  01                                    |.               | TitleIndex = 1 (age)
00000107  08                                    |.               | FieldLen = 8
00000108  2800000000000000                      |(.......        | Data = 40
00000110  02                                    |.               | TitleIndex = 2 (active)
00000111  01                                    |.               | FieldLen = 1
00000112  01                                    |.               | Data = True
00000113  03                                    |.               | TitleIndex = 3 (name)
00000114  0d00                                  |..              | FieldLen = 13
00000116  4361726f6c204a656e6b696e73            |Carol Jenkins   | Data = "Carol Jenkins"
00000123  04                                    |.               | TitleIndex = 4 (friends)
00000124  50000000                              |P...            | FieldLen = 80
00000128  5b226461766964222c2022657665222c2022  |["david", "eve", "
00000142  66726564222c202267726567222c20226861  |fred", "greg", "ha
0000015c  727279222c202269616e222c20226a616e65  |rry", "ian", "jane
00000176  222c20226b617465222c20226c696c79222c  |", "kate", "lily",
00000190  20226d617279225d                      | "mary"]        | Data = ["david","eve",...,"mary"] (10 items)
00000196  fdfeff                                |...             | FEE = 0xFDFEFF

▼ CRC (4 bytes)
00000199  fe6d465c                              |.mF\            | CRC32 = 0xFE6D465C
```