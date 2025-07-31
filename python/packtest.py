import random
from statistics import median
from faker import Faker
from bsdmp import BSDMPFieldSize, BSDMPFieldType, BSDMPPack

fake = Faker()

TYPES = [
    BSDMPFieldType.STRING,
    BSDMPFieldType.INT,
    BSDMPFieldType.FLOAT,
    BSDMPFieldType.BOOL,
    BSDMPFieldType.JSON,
]

SIZES = [
    BSDMPFieldSize.B255,
    BSDMPFieldSize.K64,
    BSDMPFieldSize.G4,
]


def random_value(type_code):
    if type_code == BSDMPFieldType.STRING:
        return fake.word()
    elif type_code == BSDMPFieldType.INT:
        return random.randint(-1000, 1000)
    elif type_code == BSDMPFieldType.FLOAT:
        return random.uniform(-1000, 1000)
    elif type_code == BSDMPFieldType.BOOL:
        return random.choice([True, False])
    elif type_code == BSDMPFieldType.JSON:
        return [fake.word() for _ in range(random.randint(5, 50))]
    else:
        return None


def generate_columns():
    count = random.randint(15, 30)
    cols = []
    for _ in range(count):
        t = random.choice(TYPES)
        s = random.choice(SIZES)
        if t == BSDMPFieldType.JSON:
            s = BSDMPFieldSize.G4
        name = fake.word()
        cols.append((name, t, s))
    return cols


def generate_rows(columns, count=100):
    rows = []
    for _ in range(count):
        row = {}
        for name, t, s in columns:
            row[name] = random_value(t)
        rows.append(row)
    return rows


def run_benchmark(max_compression=4, max_rows=100):
    columns = generate_columns()
    rows = generate_rows(columns, max_rows)

    results = {
        c: {n: [] for n in range(1, max_rows + 1)} for c in range(max_compression + 1)
    }

    for compression in range(max_compression + 1):
        for count in range(1, max_rows + 1):
            msg = BSDMPPack(compression=compression)
            msg.title(columns)
            for r in rows[:count]:
                msg.frame(r)
            packed = msg.pack()
            results[compression][count].append(len(packed))

    summary = {}
    for compression in results:
        summary[compression] = {}
        for cnt in [10, 25, 50, 95]:
            sizes = results[compression][cnt]
            if sizes:
                summary[compression][cnt] = median(sizes)
            else:
                summary[compression][cnt] = None

    return summary, columns, rows


if __name__ == "__main__":
    summary, columns, rows = run_benchmark()

    print("Колонки:")
    for name, t, s in columns:
        print(f"- {name}: type={t}, size={s}")

    print("\nРазмеры (медиана) для count в [10,30,50,80]:")
    for comp in sorted(summary.keys()):
        print(f"Compression={comp}:")
        for cnt in sorted(summary[comp].keys()):
            print(f"  count={cnt}: size={summary[comp][cnt]} байт")
