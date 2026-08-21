#!/usr/bin/env dart

/// Generate the Atlas logo from exact equilateral-triangle geometry.
library;

import 'dart:io';
import 'dart:math' as math;
import 'dart:typed_data';

typedef Point = (double, double);
typedef Polygon = List<Point>;

const size = 512;
const supersample = 4;
// Keep about 15% padding around the stroked mark so launcher masks
// do not clip the apex or base corners.
const side = 280.0;
const halfStroke = 21.0;

/// Ayu Dark editor background from Zed's `ayu.json` / Atlas Flutter canvas.
const ayuDarkBackground = (13, 16, 22);

double _x(Point point) => point.$1;
double _y(Point point) => point.$2;

Point add(Point a, Point b) => (a.$1 + b.$1, a.$2 + b.$2);

Point subtract(Point a, Point b) => (a.$1 - b.$1, a.$2 - b.$2);

Point scale(Point vector, double factor) =>
    (vector.$1 * factor, vector.$2 * factor);

double cross(Point a, Point b) => a.$1 * b.$2 - a.$2 * b.$1;

double hypot(Point vector) =>
    math.sqrt(vector.$1 * vector.$1 + vector.$2 * vector.$2);

double distance(Point a, Point b) => hypot(subtract(a, b));

Point midpoint(Point a, Point b) => ((a.$1 + b.$1) / 2.0, (a.$2 + b.$2) / 2.0);

Point centroid(List<Point> points) {
  var x = 0.0;
  var y = 0.0;
  for (final point in points) {
    x += point.$1;
    y += point.$2;
  }
  return (x / points.length, y / points.length);
}

bool isClose(double a, double b, {required double absTol}) =>
    (a - b).abs() <= absTol;

/// Returns the unit normal that points from an edge into its triangle.
Point inwardNormal(Point start, Point end, Point interior) {
  final direction = subtract(end, start);
  final length = hypot(direction);
  var normal = (-direction.$2 / length, direction.$1 / length);
  final edgeMidpoint = midpoint(start, end);
  final towardInterior = subtract(interior, edgeMidpoint);
  if (normal.$1 * towardInterior.$1 + normal.$2 * towardInterior.$2 < 0.0) {
    normal = scale(normal, -1.0);
  }
  return normal;
}

/// Returns the intersection of two non-parallel infinite lines.
Point lineIntersection(
  Point originA,
  Point directionA,
  Point originB,
  Point directionB,
) {
  final denominator = cross(directionA, directionB);
  if (isClose(denominator, 0.0, absTol: 1e-12)) {
    throw StateError('parallel lines cannot form a logo corner');
  }
  final distanceAlong =
      cross(subtract(originB, originA), directionB) / denominator;
  return add(originA, scale(directionA, distanceAlong));
}

/// Constructs a centered, sharp-mitered stroke with triangular-grid cuts.
Polygon centeredPolyline({
  required List<Point> points,
  required Point startCap,
  required Point endCap,
  required double halfWidth,
  Point? startCapOrigin,
  Point? endCapOrigin,
}) {
  final capStart = startCapOrigin ?? points.first;
  final capEnd = endCapOrigin ?? points.last;
  final sides = <List<Point>>[[], []];
  for (final (sideIndex, sign) in [(0, 1.0), (1, -1.0)]) {
    final offsetLines = <(Point, Point)>[];
    for (var i = 0; i < points.length - 1; i++) {
      final start = points[i];
      final end = points[i + 1];
      final direction = subtract(end, start);
      final normal = inwardNormal(start, end, centroid(points));
      offsetLines.add((add(start, scale(normal, sign * halfWidth)), direction));
    }
    final (firstOrigin, firstDirection) = offsetLines.first;
    sides[sideIndex].add(
      lineIntersection(firstOrigin, firstDirection, capStart, startCap),
    );
    for (var i = 0; i < offsetLines.length - 1; i++) {
      final previous = offsetLines[i];
      final current = offsetLines[i + 1];
      sides[sideIndex].add(
        lineIntersection(previous.$1, previous.$2, current.$1, current.$2),
      );
    }
    final (lastOrigin, lastDirection) = offsetLines.last;
    sides[sideIndex].add(
      lineIntersection(lastOrigin, lastDirection, capEnd, endCap),
    );
  }
  return [...sides[0], ...sides[1].reversed];
}

/// Offsets all triangle edges inward by a signed perpendicular distance.
Polygon insetTriangle((Point, Point, Point) triangle, double inset) {
  final points = [triangle.$1, triangle.$2, triangle.$3];
  final interior = centroid(points);
  final lines = <(Point, Point)>[];
  for (var i = 0; i < points.length; i++) {
    final start = points[i];
    final end = points[(i + 1) % points.length];
    final direction = subtract(end, start);
    final offset = scale(inwardNormal(start, end, interior), inset);
    lines.add((add(start, offset), direction));
  }
  final result = <Point>[];
  for (var i = 0; i < lines.length; i++) {
    final previous = lines[(i - 1) % lines.length];
    final current = lines[i];
    result.add(
      lineIntersection(previous.$1, previous.$2, current.$1, current.$2),
    );
  }
  return result;
}

/// Constructs aligned main-mountain bands and the folded mountain ring.
((Point, Point, Point), List<Polygon>, List<Polygon>) logoGeometry() {
  final height = side * math.sqrt(3.0) / 2.0;
  final apexY = (size - height) / 2.0;
  final apex = (size / 2.0, apexY);
  final left = ((size - side) / 2.0, apexY + height);
  final right = ((size + side) / 2.0, apexY + height);
  final breakPoint = midpoint(apex, right);
  final baseMidpoint = midpoint(left, right);

  final triangle = (apex, left, right);
  final rightEdge = subtract(right, apex);
  final foldedTriangle = (breakPoint, baseMidpoint, right);
  final outerTriangle = insetTriangle(triangle, -halfStroke);
  final filled = [
    centeredPolyline(
      points: [apex, left, right],
      startCap: rightEdge,
      endCap: rightEdge,
      halfWidth: halfStroke,
      startCapOrigin: outerTriangle[0],
      endCapOrigin: outerTriangle[2],
    ),
    insetTriangle(foldedTriangle, -halfStroke),
  ];
  final holes = [insetTriangle(foldedTriangle, halfStroke)];
  return (triangle, filled, holes);
}

/// Asserts equal sides, the 120-degree fold, and triangular-grid edges.
void validateGeometry(
  (Point, Point, Point) triangle,
  List<Polygon> filled,
  List<Polygon> holes,
) {
  final (apex, left, right) = triangle;
  final lengths = [
    distance(apex, left),
    distance(left, right),
    distance(right, apex),
  ];
  if (lengths.reduce(math.max) - lengths.reduce(math.min) > 1e-9) {
    throw StateError('triangle sides differ: $lengths');
  }

  final breakPoint = midpoint(apex, right);
  final baseMidpoint = midpoint(left, right);
  final foldedLengths = [
    distance(breakPoint, baseMidpoint),
    distance(baseMidpoint, right),
    distance(right, breakPoint),
  ];
  if (foldedLengths.reduce(math.max) - foldedLengths.reduce(math.min) > 1e-9) {
    throw StateError('folded triangle sides differ: $foldedLengths');
  }

  final original = subtract(apex, breakPoint);
  final folded = subtract(baseMidpoint, breakPoint);
  final cosine =
      (original.$1 * folded.$1 + original.$2 * folded.$2) /
      (hypot(original) * hypot(folded));
  final angle = math.acos(cosine.clamp(-1.0, 1.0)) * 180.0 / math.pi;
  if (!isClose(angle, 120.0, absTol: 1e-9)) {
    throw StateError('fold angle is $angle, expected 120 degrees');
  }

  final outerApex = filled[1][0];
  final innerApex = holes[0][0];
  final centerApex = midpoint(outerApex, innerApex);
  if (distance(centerApex, breakPoint) > 1e-9) {
    throw StateError(
      'folded stroke is not centered on the outer edge midpoint',
    );
  }

  final outerTriangle = insetTriangle(triangle, -halfStroke);
  final nearest = filled[0]
      .map((point) => distance(point, outerTriangle[0]))
      .reduce(math.min);
  if (nearest > 1e-9) {
    throw StateError('main left edge does not reach the outer triangle apex');
  }
  final foldedOuter = filled[1];
  final foldedRight = subtract(foldedOuter[2], foldedOuter[0]);
  if (cross(foldedRight, subtract(outerTriangle[0], foldedOuter[0])).abs() >
      1e-7) {
    throw StateError(
      'folded right edge does not align with the outer triangle',
    );
  }

  const allowed = [0.0, 60.0, 120.0];
  for (final polygon in [...filled, ...holes]) {
    for (var i = 0; i < polygon.length; i++) {
      final start = polygon[i];
      final end = polygon[(i + 1) % polygon.length];
      final vector = subtract(end, start);
      final edgeAngle =
          (math.atan2(vector.$2, vector.$1) * 180.0 / math.pi) % 180.0;
      final closest = allowed
          .map((candidate) => (edgeAngle - candidate).abs())
          .reduce(math.min);
      if (closest > 1e-7) {
        throw StateError(
          'edge angle $edgeAngle is outside the triangular grid',
        );
      }
    }
  }
}

/// Rasterizes a polygon into the supersampled mask using even-odd filling.
void fillPolygon(Uint8List mask, Polygon polygon, int canvasSize, int value) {
  var minY = canvasSize;
  var maxY = 0;
  for (final point in polygon) {
    minY = math.min(minY, _y(point).floor());
    maxY = math.max(maxY, _y(point).ceil());
  }
  minY = math.max(0, minY);
  maxY = math.min(canvasSize - 1, maxY);

  for (var y = minY; y <= maxY; y++) {
    final scanY = y + 0.5;
    final intersections = <double>[];
    for (var i = 0; i < polygon.length; i++) {
      final start = polygon[i];
      final end = polygon[(i + 1) % polygon.length];
      if ((_y(start) <= scanY && scanY < _y(end)) ||
          (_y(end) <= scanY && scanY < _y(start))) {
        final ratio = (scanY - _y(start)) / (_y(end) - _y(start));
        intersections.add(_x(start) + ratio * (_x(end) - _x(start)));
      }
    }
    if (intersections.length < 2) {
      continue;
    }
    intersections.sort();
    final rowStart = y * canvasSize;
    for (var i = 0; i + 1 < intersections.length; i += 2) {
      final xStart = math.max(0, intersections[i].floor());
      final xEnd = math.min(canvasSize, intersections[i + 1].ceil());
      mask.fillRange(rowStart + xStart, rowStart + xEnd, value);
    }
  }
}

final _crcTable = List<int>.generate(256, (n) {
  var crc = n;
  for (var i = 0; i < 8; i++) {
    crc = (crc & 1) != 0 ? (crc >> 1) ^ 0xEDB88320 : crc >> 1;
  }
  return crc;
});

int _crc32(List<int> data) {
  var crc = 0xFFFFFFFF;
  for (final byte in data) {
    crc = _crcTable[(crc ^ byte) & 0xFF] ^ (crc >>> 8);
  }
  return crc ^ 0xFFFFFFFF;
}

/// Rounds like Python's `round`: halves go to the nearest even integer.
int _roundHalfEven(double value) {
  final lower = value.floor();
  final fraction = value - lower;
  if (fraction < 0.5) return lower;
  if (fraction > 0.5) return lower + 1;
  return lower.isEven ? lower : lower + 1;
}

Uint8List _pngChunk(List<int> kind, List<int> data) {
  final payload = Uint8List(4 + kind.length + data.length + 4);
  final view = ByteData.sublistView(payload);
  view.setUint32(0, data.length);
  payload.setRange(4, 4 + kind.length, kind);
  payload.setRange(4 + kind.length, 4 + kind.length + data.length, data);
  view.setUint32(4 + kind.length + data.length, _crc32([...kind, ...data]));
  return payload;
}

/// Renders the Ayu Dark background and antialiased white mark to PNG bytes.
Uint8List encodePng(List<Polygon> filled, List<Polygon> holes, int pixelSize) {
  final sampleSize = pixelSize * supersample;
  final factor = sampleSize / size;
  final mask = Uint8List(sampleSize * sampleSize);
  Polygon scaled(Polygon polygon) => [
    for (final point in polygon) (point.$1 * factor, point.$2 * factor),
  ];
  for (final polygon in filled) {
    fillPolygon(mask, scaled(polygon), sampleSize, 255);
  }
  for (final polygon in holes) {
    fillPolygon(mask, scaled(polygon), sampleSize, 0);
  }

  final rows = BytesBuilder(copy: false);
  final sampleCount = supersample * supersample;
  for (var y = 0; y < pixelSize; y++) {
    rows.addByte(0);
    for (var x = 0; x < pixelSize; x++) {
      var coverage = 0;
      for (var offsetY = 0; offsetY < supersample; offsetY++) {
        final row = (y * supersample + offsetY) * sampleSize;
        final start = row + x * supersample;
        for (var offsetX = 0; offsetX < supersample; offsetX++) {
          coverage += mask[start + offsetX];
        }
      }
      final alpha = coverage / (255.0 * sampleCount);
      rows.addByte(
        _roundHalfEven(ayuDarkBackground.$1 * (1.0 - alpha) + 255.0 * alpha),
      );
      rows.addByte(
        _roundHalfEven(ayuDarkBackground.$2 * (1.0 - alpha) + 255.0 * alpha),
      );
      rows.addByte(
        _roundHalfEven(ayuDarkBackground.$3 * (1.0 - alpha) + 255.0 * alpha),
      );
    }
  }

  final header = ByteData(13)
    ..setUint32(0, pixelSize)
    ..setUint32(4, pixelSize)
    ..setUint8(8, 8)
    ..setUint8(9, 2)
    ..setUint8(10, 0)
    ..setUint8(11, 0)
    ..setUint8(12, 0);
  final compressed = ZLibCodec(level: 9).encode(rows.takeBytes());
  final png = BytesBuilder(copy: false)
    ..add([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])
    ..add(_pngChunk([0x49, 0x48, 0x44, 0x52], header.buffer.asUint8List()))
    ..add(_pngChunk([0x49, 0x44, 0x41, 0x54], compressed))
    ..add(_pngChunk([0x49, 0x45, 0x4E, 0x44], const []));
  return png.takeBytes();
}

void _writePng(
  File output,
  List<Polygon> filled,
  List<Polygon> holes, [
  int pixelSize = size,
]) {
  output.parent.createSync(recursive: true);
  output.writeAsBytesSync(encodePng(filled, holes, pixelSize));
}

/// Writes a Windows ICO containing PNG-compressed square images.
void _writeIco(File output, List<(int, Uint8List)> images) {
  var offset = 6 + 16 * images.length;
  final header = ByteData(6)
    ..setUint16(0, 0, Endian.little)
    ..setUint16(2, 1, Endian.little)
    ..setUint16(4, images.length, Endian.little);
  final entries = BytesBuilder(copy: false);
  final payloads = BytesBuilder(copy: false);
  for (final (imageSize, png) in images) {
    final entry = ByteData(16)
      ..setUint8(0, imageSize >= 256 ? 0 : imageSize)
      ..setUint8(1, imageSize >= 256 ? 0 : imageSize)
      ..setUint8(2, 0)
      ..setUint8(3, 0)
      ..setUint16(4, 1, Endian.little)
      ..setUint16(6, 32, Endian.little)
      ..setUint32(8, png.length, Endian.little)
      ..setUint32(12, offset, Endian.little);
    entries.add(entry.buffer.asUint8List());
    payloads.add(png);
    offset += png.length;
  }
  output.parent.createSync(recursive: true);
  output.writeAsBytesSync([
    ...header.buffer.asUint8List(),
    ...entries.takeBytes(),
    ...payloads.takeBytes(),
  ]);
}

/// Rasterizes launcher icons for every Flutter platform.
List<File> _writeAppIcons(
  Directory root,
  List<Polygon> filled,
  List<Polygon> holes,
) {
  final cache = <int, Uint8List>{};
  Uint8List pngAt(int pixelSize) =>
      cache.putIfAbsent(pixelSize, () => encodePng(filled, holes, pixelSize));

  File writeAt(String relativePath, int pixelSize) {
    final file = File('${root.path}/$relativePath');
    file.parent.createSync(recursive: true);
    file.writeAsBytesSync(pngAt(pixelSize));
    return file;
  }

  const targets = <(String, int)>[
    ('docs/assets/atlas-logo-512.png', 512),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_16.png',
      16,
    ),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_32.png',
      32,
    ),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_64.png',
      64,
    ),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_128.png',
      128,
    ),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_256.png',
      256,
    ),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_512.png',
      512,
    ),
    (
      'apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_1024.png',
      1024,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-20x20@1x.png',
      20,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-20x20@2x.png',
      40,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-20x20@3x.png',
      60,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-29x29@1x.png',
      29,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-29x29@2x.png',
      58,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-29x29@3x.png',
      87,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-40x40@1x.png',
      40,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-40x40@2x.png',
      80,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-40x40@3x.png',
      120,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-60x60@2x.png',
      120,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-60x60@3x.png',
      180,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-76x76@1x.png',
      76,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-76x76@2x.png',
      152,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-83.5x83.5@2x.png',
      167,
    ),
    (
      'apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-1024x1024@1x.png',
      1024,
    ),
    (
      'apps/atlas_flutter/android/app/src/main/res/mipmap-mdpi/ic_launcher.png',
      48,
    ),
    (
      'apps/atlas_flutter/android/app/src/main/res/mipmap-hdpi/ic_launcher.png',
      72,
    ),
    (
      'apps/atlas_flutter/android/app/src/main/res/mipmap-xhdpi/ic_launcher.png',
      96,
    ),
    (
      'apps/atlas_flutter/android/app/src/main/res/mipmap-xxhdpi/ic_launcher.png',
      144,
    ),
    (
      'apps/atlas_flutter/android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png',
      192,
    ),
    ('apps/atlas_flutter/linux/runner/resources/app_icon.png', 512),
  ];
  final written = [
    for (final (path, pixelSize) in targets) writeAt(path, pixelSize),
  ];
  final ico = File(
    '${root.path}/apps/atlas_flutter/windows/runner/resources/app_icon.ico',
  );
  _writeIco(ico, [
    for (final s in const [16, 32, 48, 256]) (s, pngAt(s)),
  ]);
  written.add(ico);
  return written;
}

void main(List<String> args) {
  final root = File.fromUri(Platform.script).parent.parent;
  var outputPath = '';
  var outputSize = size;
  for (var i = 0; i < args.length; i++) {
    switch (args[i]) {
      case '--output':
        outputPath = args[++i];
      case '--size':
        outputSize = int.parse(args[++i]);
      default:
        throw ArgumentError('unknown argument: ${args[i]}');
    }
  }

  final (triangle, filled, holes) = logoGeometry();
  validateGeometry(triangle, filled, holes);
  if (outputPath.isNotEmpty) {
    _writePng(File(outputPath), filled, holes, outputSize);
    print('Generated $outputPath with exact 60-degree geometry');
    return;
  }
  final written = _writeAppIcons(root, filled, holes);
  print('Generated ${written.length} logo and launcher icon files');
}
