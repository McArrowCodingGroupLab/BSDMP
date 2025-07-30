<?php

require_once './bsdmp.php';

// Пример клиента
$client = new BSDMPClient(version: 1, compression: BSDMP::COMPRESSION_GZIP);
$client->format(['name', 'age', 'active']);
$client->addFrame(['Alice', 25, true]);
$client->addFrame(['Bob', 30, false]);

$encodedData = $client->encode();
echo "Encoded data (hex): " . bin2hex($encodedData) . "\n\n";

$encodedData = hex2bin('0100000001000000780000001f8b08000000000000ff348c410a023110046bb249fc81fa0c51bc8910416f5efcc118460944058dbe5f24eea569e8ae7240040ea774dc6f27c0a6d729e0b9ebcd6440af2611cdad7c4cfa2ec01c08a45ab239966b4f7bbeedcff7fce967c0c0ee7176ac16818bd6d778fa060000ffff97d8e9e87e000000');

// Пример сервера
$server = new BSDMPServer();
$server->decode($encodedData);
foreach ($server->frames as $i => $frame) {
    echo "=== Frame " . ($i + 1) . " ===\n";
    foreach ($frame as $key => $value) {
        if (is_string($value) && !mb_check_encoding($value, 'UTF-8')) {
            echo $key . '=0x' . bin2hex($value) . "\n";
        } else {
            echo $key . '=' . (is_array($value) ? json_encode($value) : $value) . "\n";
        }
    }
    echo "\n";
}
