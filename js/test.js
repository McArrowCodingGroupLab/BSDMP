import { Pack, Unpack, FieldType, FieldSize, CompressionMethod } from './bsdmp.js';

// Создание пакета
const pack = new Pack(CompressionMethod.GZIP);
pack.title([
    ['name', FieldType.STRING, FieldSize.B255],
    ['age', FieldType.INT, FieldSize.G4],
    ['active', FieldType.BOOL, FieldSize.B255],
]);
pack.frame({ name: 'Alice', active: true });
pack.frame({ name: 'Bob', age: 30 });
const data = pack.pack();
// показываем как hex
console.log(data.toString('hex'));
// Распаковка
const unpack = new Unpack(data, true);
console.log(unpack.frames);