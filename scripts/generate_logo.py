#!/usr/bin/env python3
"""Generate the Atlas logo from exact equilateral-triangle geometry."""

from __future__ import annotations

import argparse
import math
import struct
import zlib
from pathlib import Path

Point = tuple[float, float]
Polygon = list[Point]

SIZE = 512
SUPERSAMPLE = 4
# Keep about 20% padding around the stroked mark so launcher masks
# do not clip the apex or base corners.
SIDE = 248.0
HALF_STROKE = 18.6


def cross(a: Point, b: Point) -> float:
    """Return the two-dimensional cross product of two vectors."""
    return a[0] * b[1] - a[1] * b[0]


def subtract(a: Point, b: Point) -> Point:
    """Return vector a-b."""
    return a[0] - b[0], a[1] - b[1]


def midpoint(a: Point, b: Point) -> Point:
    """Return the midpoint between two points."""
    return (a[0] + b[0]) / 2.0, (a[1] + b[1]) / 2.0


def centroid(points: list[Point] | tuple[Point, ...]) -> Point:
    """Return the centroid of a set of points."""
    return (
        sum(point[0] for point in points) / len(points),
        sum(point[1] for point in points) / len(points),
    )


def add(a: Point, b: Point) -> Point:
    """Return point or vector a+b."""
    return a[0] + b[0], a[1] + b[1]


def scale(vector: Point, factor: float) -> Point:
    """Scale a vector by a scalar."""
    return vector[0] * factor, vector[1] * factor


def inward_normal(start: Point, end: Point, interior: Point) -> Point:
    """Return the unit normal that points from an edge into its triangle."""
    direction = subtract(end, start)
    length = math.hypot(*direction)
    normal = (-direction[1] / length, direction[0] / length)
    edge_midpoint = midpoint(start, end)
    toward_interior = subtract(interior, edge_midpoint)
    if normal[0] * toward_interior[0] + normal[1] * toward_interior[1] < 0.0:
        normal = scale(normal, -1.0)
    return normal


def line_intersection(origin_a: Point, direction_a: Point, origin_b: Point, direction_b: Point) -> Point:
    """Return the intersection of two non-parallel infinite lines."""
    denominator = cross(direction_a, direction_b)
    if math.isclose(denominator, 0.0, abs_tol=1e-12):
        raise ValueError("parallel lines cannot form a logo corner")
    distance = cross(subtract(origin_b, origin_a), direction_b) / denominator
    return add(origin_a, scale(direction_a, distance))


def centered_polyline(
    points: list[Point],
    start_cap: Point,
    end_cap: Point,
    half_width: float,
    start_cap_origin: Point | None = None,
    end_cap_origin: Point | None = None,
) -> Polygon:
    """Construct a centered, sharp-mitered stroke with triangular-grid cuts."""
    start_cap_origin = start_cap_origin or points[0]
    end_cap_origin = end_cap_origin or points[-1]
    sides: list[list[Point]] = [[], []]
    for side_index, sign in enumerate((1.0, -1.0)):
        offset_lines: list[tuple[Point, Point]] = []
        for start, end in zip(points, points[1:]):
            direction = subtract(end, start)
            normal = inward_normal(start, end, centroid(points))
            offset_lines.append((add(start, scale(normal, sign * half_width)), direction))

        first_origin, first_direction = offset_lines[0]
        sides[side_index].append(
            line_intersection(first_origin, first_direction, start_cap_origin, start_cap)
        )
        for previous, current in zip(offset_lines, offset_lines[1:]):
            sides[side_index].append(line_intersection(previous[0], previous[1], current[0], current[1]))
        last_origin, last_direction = offset_lines[-1]
        sides[side_index].append(
            line_intersection(last_origin, last_direction, end_cap_origin, end_cap)
        )

    return sides[0] + list(reversed(sides[1]))


def inset_triangle(triangle: tuple[Point, Point, Point], distance: float) -> Polygon:
    """Offset all triangle edges inward by a signed perpendicular distance."""
    interior = centroid(triangle)
    lines: list[tuple[Point, Point]] = []
    for start, end in zip(triangle, triangle[1:] + triangle[:1]):
        direction = subtract(end, start)
        offset = scale(inward_normal(start, end, interior), distance)
        lines.append((add(start, offset), direction))

    result: Polygon = []
    for previous, current in zip(lines[-1:] + lines[:-1], lines):
        result.append(line_intersection(previous[0], previous[1], current[0], current[1]))
    return result


def logo_geometry() -> tuple[tuple[Point, Point, Point], list[Polygon], list[Polygon]]:
    """Construct aligned main-mountain bands and the folded mountain ring."""
    height = SIDE * math.sqrt(3.0) / 2.0
    apex_y = (SIZE - height) / 2.0
    apex = (SIZE / 2.0, apex_y)
    left = ((SIZE - SIDE) / 2.0, apex_y + height)
    right = ((SIZE + SIDE) / 2.0, apex_y + height)
    break_point = midpoint(apex, right)
    base_midpoint = midpoint(left, right)

    triangle = (apex, left, right)
    right_edge = subtract(right, apex)

    folded_triangle = (break_point, base_midpoint, right)
    outer_triangle = inset_triangle(triangle, -HALF_STROKE)
    filled = [
        centered_polyline(
            [apex, left, right],
            right_edge,
            right_edge,
            HALF_STROKE,
            start_cap_origin=outer_triangle[0],
            end_cap_origin=outer_triangle[2],
        ),
        inset_triangle(folded_triangle, -HALF_STROKE),
    ]
    holes = [inset_triangle(folded_triangle, HALF_STROKE)]
    return triangle, filled, holes


def validate_geometry(
    triangle: tuple[Point, Point, Point],
    filled: list[Polygon],
    holes: list[Polygon],
) -> None:
    """Assert equal sides, the 120-degree fold, and triangular-grid edges."""
    apex, left, right = triangle
    lengths = [
        math.dist(apex, left),
        math.dist(left, right),
        math.dist(right, apex),
    ]
    if max(lengths) - min(lengths) > 1e-9:
        raise ValueError(f"triangle sides differ: {lengths}")

    break_point = midpoint(apex, right)
    base_midpoint = midpoint(left, right)
    folded_lengths = [
        math.dist(break_point, base_midpoint),
        math.dist(base_midpoint, right),
        math.dist(right, break_point),
    ]
    if max(folded_lengths) - min(folded_lengths) > 1e-9:
        raise ValueError(f"folded triangle sides differ: {folded_lengths}")

    original = subtract(apex, break_point)
    folded = subtract(base_midpoint, break_point)
    cosine = sum(a * b for a, b in zip(original, folded)) / (
        math.hypot(*original) * math.hypot(*folded)
    )
    angle = math.degrees(math.acos(max(-1.0, min(1.0, cosine))))
    if not math.isclose(angle, 120.0, abs_tol=1e-9):
        raise ValueError(f"fold angle is {angle}, expected 120 degrees")

    outer_apex = filled[1][0]
    inner_apex = holes[0][0]
    center_apex = midpoint(outer_apex, inner_apex)
    if math.dist(center_apex, break_point) > 1e-9:
        raise ValueError("folded stroke is not centered on the outer edge midpoint")

    outer_triangle = inset_triangle(triangle, -HALF_STROKE)
    if min(math.dist(point, outer_triangle[0]) for point in filled[0]) > 1e-9:
        raise ValueError("main left edge does not reach the outer triangle apex")
    folded_outer = filled[1]
    folded_right = subtract(folded_outer[2], folded_outer[0])
    if abs(cross(folded_right, subtract(outer_triangle[0], folded_outer[0]))) > 1e-7:
        raise ValueError("folded right edge does not align with the outer triangle")

    allowed = (0.0, 60.0, 120.0)
    for polygon in filled + holes:
        for start, end in zip(polygon, polygon[1:] + polygon[:1]):
            vector = subtract(end, start)
            edge_angle = math.degrees(math.atan2(vector[1], vector[0])) % 180.0
            if min(abs(edge_angle - candidate) for candidate in allowed) > 1e-7:
                raise ValueError(f"edge angle {edge_angle} is outside the triangular grid")


def fill_polygon(mask: bytearray, polygon: Polygon, canvas_size: int, value: int) -> None:
    """Rasterize a polygon into the supersampled mask using even-odd filling."""
    min_y = max(0, int(math.floor(min(y for _, y in polygon))))
    max_y = min(canvas_size - 1, int(math.ceil(max(y for _, y in polygon))))

    for y in range(min_y, max_y + 1):
        scan_y = y + 0.5
        intersections: list[float] = []
        for start, end in zip(polygon, polygon[1:] + polygon[:1]):
            if (start[1] <= scan_y < end[1]) or (end[1] <= scan_y < start[1]):
                ratio = (scan_y - start[1]) / (end[1] - start[1])
                intersections.append(start[0] + ratio * (end[0] - start[0]))
        if len(intersections) < 2:
            continue
        intersections.sort()
        row_start = y * canvas_size
        for left, right in zip(intersections[::2], intersections[1::2]):
            x_start = max(0, int(math.floor(left)))
            x_end = min(canvas_size, int(math.ceil(right)))
            mask[row_start + x_start : row_start + x_end] = bytes((value,)) * (x_end - x_start)


# Ayu Dark editor background from Zed's `ayu.json` / Atlas Flutter canvas.
AYU_DARK_BACKGROUND = (13, 16, 22)


def png_chunk(kind: bytes, data: bytes) -> bytes:
    """Encode one PNG chunk."""
    checksum = zlib.crc32(kind)
    checksum = zlib.crc32(data, checksum)
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", checksum & 0xFFFFFFFF)


def encode_png(filled: list[Polygon], holes: list[Polygon], size: int) -> bytes:
    """Render the Ayu Dark background and antialiased white mark to PNG bytes."""
    sample_size = size * SUPERSAMPLE
    factor = sample_size / SIZE
    mask = bytearray(sample_size * sample_size)
    for polygon in filled:
        fill_polygon(
            mask,
            [(x * factor, y * factor) for x, y in polygon],
            sample_size,
            255,
        )
    for polygon in holes:
        fill_polygon(
            mask,
            [(x * factor, y * factor) for x, y in polygon],
            sample_size,
            0,
        )

    rows = bytearray()
    sample_count = SUPERSAMPLE * SUPERSAMPLE
    for y in range(size):
        rows.append(0)
        for x in range(size):
            coverage = 0
            for offset_y in range(SUPERSAMPLE):
                row = (y * SUPERSAMPLE + offset_y) * sample_size
                start = row + x * SUPERSAMPLE
                coverage += sum(mask[start : start + SUPERSAMPLE])
            alpha = coverage / (255.0 * sample_count)
            rows.extend(
                round(channel * (1.0 - alpha) + 255.0 * alpha)
                for channel in AYU_DARK_BACKGROUND
            )

    header = struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0)
    png = b"\x89PNG\r\n\x1a\n"
    png += png_chunk(b"IHDR", header)
    png += png_chunk(b"IDAT", zlib.compress(bytes(rows), level=9))
    png += png_chunk(b"IEND", b"")
    return png


def write_png(output: Path, filled: list[Polygon], holes: list[Polygon], size: int = SIZE) -> None:
    """Write one square PNG of [size] pixels."""
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(encode_png(filled, holes, size))


def write_ico(output: Path, images: list[tuple[int, bytes]]) -> None:
    """Write a Windows ICO containing PNG-compressed square images."""
    offset = 6 + 16 * len(images)
    header = struct.pack("<HHH", 0, 1, len(images))
    entries = bytearray()
    payloads = bytearray()
    for size, png in images:
        width = 0 if size >= 256 else size
        height = 0 if size >= 256 else size
        entries += struct.pack("<BBBBHHII", width, height, 0, 0, 1, 32, len(png), offset)
        payloads += png
        offset += len(png)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(header + entries + payloads)


def write_app_icons(root: Path, filled: list[Polygon], holes: list[Polygon]) -> list[Path]:
    """Rasterize launcher icons for every Flutter platform."""
    cache: dict[int, bytes] = {}

    def png_at(size: int) -> bytes:
        if size not in cache:
            cache[size] = encode_png(filled, holes, size)
        return cache[size]

    def write_at(path: Path, size: int) -> Path:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(png_at(size))
        return path

    written = [
        write_at(root / "docs/assets/atlas-logo-512.png", 512),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_16.png",
            16,
        ),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_32.png",
            32,
        ),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_64.png",
            64,
        ),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_128.png",
            128,
        ),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_256.png",
            256,
        ),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_512.png",
            512,
        ),
        write_at(
            root / "apps/atlas_flutter/macos/Runner/Assets.xcassets/AppIcon.appiconset/app_icon_1024.png",
            1024,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-20x20@1x.png",
            20,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-20x20@2x.png",
            40,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-20x20@3x.png",
            60,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-29x29@1x.png",
            29,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-29x29@2x.png",
            58,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-29x29@3x.png",
            87,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-40x40@1x.png",
            40,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-40x40@2x.png",
            80,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-40x40@3x.png",
            120,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-60x60@2x.png",
            120,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-60x60@3x.png",
            180,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-76x76@1x.png",
            76,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-76x76@2x.png",
            152,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-83.5x83.5@2x.png",
            167,
        ),
        write_at(
            root / "apps/atlas_flutter/ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-1024x1024@1x.png",
            1024,
        ),
        write_at(
            root / "apps/atlas_flutter/android/app/src/main/res/mipmap-mdpi/ic_launcher.png",
            48,
        ),
        write_at(
            root / "apps/atlas_flutter/android/app/src/main/res/mipmap-hdpi/ic_launcher.png",
            72,
        ),
        write_at(
            root / "apps/atlas_flutter/android/app/src/main/res/mipmap-xhdpi/ic_launcher.png",
            96,
        ),
        write_at(
            root / "apps/atlas_flutter/android/app/src/main/res/mipmap-xxhdpi/ic_launcher.png",
            144,
        ),
        write_at(
            root / "apps/atlas_flutter/android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png",
            192,
        ),
        write_at(root / "apps/atlas_flutter/linux/runner/resources/app_icon.png", 512),
    ]
    ico = root / "apps/atlas_flutter/windows/runner/resources/app_icon.ico"
    write_ico(
        ico,
        [(size, png_at(size)) for size in (16, 32, 48, 256)],
    )
    written.append(ico)
    return written


def main() -> None:
    """Generate and validate the project logo and app launcher icons."""
    root = Path(__file__).resolve().parent.parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--output",
        type=Path,
        help="Write only this PNG instead of the docs asset and app icons",
    )
    parser.add_argument(
        "--size",
        type=int,
        default=SIZE,
        help="Pixel size used with --output (default: 512)",
    )
    args = parser.parse_args()

    triangle, filled, holes = logo_geometry()
    validate_geometry(triangle, filled, holes)
    if args.output is not None:
        write_png(args.output, filled, holes, args.size)
        print(f"Generated {args.output} with exact 60-degree geometry")
        return
    written = write_app_icons(root, filled, holes)
    print(f"Generated {len(written)} logo and launcher icon files")


if __name__ == "__main__":
    main()
