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
SIDE = 320.0
APEX_Y = 104.0
HALF_STROKE = 24.0


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
    apex = (SIZE / 2.0, APEX_Y)
    left = ((SIZE - SIDE) / 2.0, APEX_Y + height)
    right = ((SIZE + SIDE) / 2.0, APEX_Y + height)
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


def fill_polygon(mask: bytearray, polygon: Polygon, scale: int, value: int) -> None:
    """Rasterize a polygon into the supersampled mask using even-odd filling."""
    scaled = [(x * scale, y * scale) for x, y in polygon]
    canvas_size = SIZE * scale
    min_y = max(0, int(math.floor(min(y for _, y in scaled))))
    max_y = min(canvas_size - 1, int(math.ceil(max(y for _, y in scaled))))

    for y in range(min_y, max_y + 1):
        scan_y = y + 0.5
        intersections: list[float] = []
        for start, end in zip(scaled, scaled[1:] + scaled[:1]):
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


def blend(base: tuple[float, float, float], color: tuple[float, float, float], alpha: float) -> tuple[float, float, float]:
    """Alpha-blend one RGB color over another."""
    return tuple(channel * (1.0 - alpha) + overlay * alpha for channel, overlay in zip(base, color))


def background_color(x: int, y: int) -> tuple[int, int, int]:
    """Return a light periwinkle, cyan, and lavender gradient color."""
    vertical = y / (SIZE - 1)
    horizontal = x / (SIZE - 1)
    top = blend((181.0, 196.0, 248.0), (142.0, 211.0, 246.0), horizontal)
    bottom = blend((111.0, 102.0, 220.0), (103.0, 127.0, 222.0), horizontal)
    smooth_vertical = vertical * vertical * (3.0 - 2.0 * vertical)
    color = blend(top, bottom, smooth_vertical)

    lavender_distance = ((horizontal - 0.48) / 0.44) ** 2 + ((vertical + 0.04) / 0.42) ** 2
    lavender_alpha = 0.24 * math.exp(-lavender_distance / 2.0)
    color = blend(color, (205.0, 186.0, 248.0), lavender_alpha)
    return tuple(max(0, min(255, round(channel))) for channel in color)


def png_chunk(kind: bytes, data: bytes) -> bytes:
    """Encode one PNG chunk."""
    checksum = zlib.crc32(kind)
    checksum = zlib.crc32(data, checksum)
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", checksum & 0xFFFFFFFF)


def write_png(output: Path, filled: list[Polygon], holes: list[Polygon]) -> None:
    """Render the gradient and antialiased white mark to an RGB PNG."""
    sample_size = SIZE * SUPERSAMPLE
    mask = bytearray(sample_size * sample_size)
    for polygon in filled:
        fill_polygon(mask, polygon, SUPERSAMPLE, 255)
    for polygon in holes:
        fill_polygon(mask, polygon, SUPERSAMPLE, 0)

    rows = bytearray()
    sample_count = SUPERSAMPLE * SUPERSAMPLE
    for y in range(SIZE):
        rows.append(0)
        for x in range(SIZE):
            coverage = 0
            for offset_y in range(SUPERSAMPLE):
                row = (y * SUPERSAMPLE + offset_y) * sample_size
                start = row + x * SUPERSAMPLE
                coverage += sum(mask[start : start + SUPERSAMPLE])
            alpha = coverage / (255.0 * sample_count)
            background = background_color(x, y)
            rows.extend(round(channel * (1.0 - alpha) + 255.0 * alpha) for channel in background)

    header = struct.pack(">IIBBBBB", SIZE, SIZE, 8, 2, 0, 0, 0)
    png = b"\x89PNG\r\n\x1a\n"
    png += png_chunk(b"IHDR", header)
    png += png_chunk(b"IDAT", zlib.compress(bytes(rows), level=9))
    png += png_chunk(b"IEND", b"")
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(png)


def main() -> None:
    """Generate and validate the project logo."""
    default_output = Path(__file__).resolve().parent.parent / "docs/assets/atlas-logo-512.png"
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=default_output)
    args = parser.parse_args()

    triangle, filled, holes = logo_geometry()
    validate_geometry(triangle, filled, holes)
    write_png(args.output, filled, holes)
    print(f"Generated {args.output} with exact 60-degree geometry")


if __name__ == "__main__":
    main()
