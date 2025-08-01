<?php

require_once 'bsdmp.php';

use BSDMP\{Pack, Unpack, FieldType, FieldSize, CompressionMethod};

// Создание пакета
$pack = new Pack(compression: CompressionMethod::NONE);
$pack->title([
    ['name', FieldType::STRING->value, FieldSize::B255->value],
    ['age', FieldType::INT->value, FieldSize::B255->value],
    ['active', FieldType::BOOL->value, FieldSize::B255->value],
]);
$pack->frame(['name' => 'Alice', 'active' => false]);
$pack->frame(['name' => 'Bob', 'age' => 30]);
$data = $pack->pack();

// Распаковка
$unpack = new Unpack($data, strict: true);
foreach ($unpack->frames as $frame) {
    var_dump($frame);
}
