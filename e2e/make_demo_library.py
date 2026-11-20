"""Writes a demo course library: believable courses with cover pictures, for the
screenshots in the README (see screenshots.sh).

Usage: python3 make_demo_library.py DIRECTORY
Needs ffmpeg (VP8) and `pip install pillow reportlab`.

Everything is made up and drawn here: the course names are generic, the
"videos" are title cards, the covers are abstract shapes. No third-party
material is involved, so the pictures can be published.
"""
import math
import os
import subprocess
import sys
import tempfile

from PIL import Image, ImageDraw, ImageFont
from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

FONT_DIRS = ['/usr/share/fonts/opentype/inter', '/usr/share/fonts/truetype/dejavu', '/Library/Fonts', '/System/Library/Fonts/Supplemental']
FONT_FILES = {'bold': ['Inter-Bold.otf', 'DejaVuSans-Bold.ttf', 'Arial Bold.ttf'], 'regular': ['Inter-Medium.otf', 'DejaVuSans.ttf', 'Arial.ttf']}


def font(kind, size):
    for d in FONT_DIRS:
        for f in FONT_FILES[kind]:
            p = os.path.join(d, f)
            if os.path.exists(p):
                return ImageFont.truetype(p, size)
    return ImageFont.load_default(size)


def hexrgb(h):
    h = h.lstrip('#')
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


# name, colours (dark, mid, light), cover motif, sections [(folder, [lesson])], a lesson is
# (title, seconds) for a video, (title, 'pdf') or (title, 'md')
COURSES = [
    dict(name='Photography Basics', colours=('#2b1d0e', '#9a5414', '#f4b942'), motif='lens', sections=[
        ('01 - Getting started', [('01 - Welcome', 96), ('02 - Know your camera', 412), ('03 - Cheat sheet', 'pdf')]),
        ('02 - Exposure', [('01 - The exposure triangle', 538), ('02 - Aperture and depth of field', 471),
                           ('03 - Shutter speed and motion', 389), ('04 - Practice shots', 'md')]),
        ('03 - Composition', [('01 - The rule of thirds', 305), ('02 - Leading lines', 264), ('03 - Light and shadow', 447)]),
    ]),
    dict(name='Python for Beginners', colours=('#0d2233', '#1b6a9a', '#7fd0f2'), motif='nodes', sections=[
        ('01 - First steps', [('01 - Install Python', 188), ('02 - Your first program', 342), ('03 - Cheat sheet', 'pdf')]),
        ('02 - Data', [('01 - Variables and types', 417), ('02 - Lists and loops', 523), ('03 - Exercises', 'md')]),
        ('03 - Functions', [('01 - Writing a function', 366), ('02 - Reading a file', 298)]),
    ]),
    dict(name='Knife Skills', colours=('#10281c', '#2f8052', '#9be3ab'), motif='stripes', sections=[
        (None, [('01 - Holding the knife', 212), ('02 - The claw grip', 176), ('03 - Dicing an onion', 331),
                ('04 - Slicing herbs', 154)]),
    ]),
    dict(name='Music Theory 101', colours=('#221336', '#6d41a6', '#cfa8f7'), motif='staff', sections=[
        ('01 - Reading music', [('01 - Notes and the staff', 281), ('02 - Rhythm and time signatures', 356), ('03 - Cheat sheet', 'pdf')]),
        ('02 - Scales and keys', [('01 - The major scale', 402), ('02 - Minor scales', 377), ('03 - Key signatures', 315)]),
    ]),
    dict(name='Watercolor Landscapes', colours=('#0d2b2e', '#2a929a', '#a5ece4'), motif='hills', sections=[
        ('01 - Materials', [('01 - Paper, paint and brushes', 244), ('02 - Mixing colours', 330)]),
        ('02 - Skies and water', [('01 - A soft sky', 418), ('02 - Still water', 391), ('03 - Reference photos', 'md')]),
    ]),
]
TALK = dict(name='Designing for Humans', colours=('#2b1022', '#a8416b', '#f7a3c0'), motif='bubbles', seconds=1470,
            chapters='0:00 Welcome\n3:20 Why nobody reads the manual\n9:45 Designing the first minute\n15:30 Testing with real people\n21:10 Questions\n')


def run(*args):
    subprocess.run(['ffmpeg', '-y', '-loglevel', 'error', *args], check=True)


def gradient(size, top, bottom):
    """Top to bottom, in two steps: one column of pixels, stretched sideways."""
    w, h = size
    col = Image.new('RGB', (1, h))
    for y in range(h):
        t = y / (h - 1)
        col.putpixel((0, y), tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3)))
    return col.resize(size)


def wrap(draw, text, fnt, width):
    lines, cur = [], ''
    for word in text.split():
        trial = (cur + ' ' + word).strip()
        if draw.textlength(trial, font=fnt) <= width or not cur:
            cur = trial
        else:
            lines.append(cur)
            cur = word
    if cur:
        lines.append(cur)
    return lines


def title_card(path, course, colours, lesson, minutes):
    """A 1280x720 picture that stands in for the video of a lesson. Positions are
    in a 640x360 grid, drawn four times as large and then shrunk to half that:
    smooth edges."""
    dark, mid, light = map(hexrgb, colours)
    s = 4
    img = gradient((640 * s, 360 * s), dark, mid).convert('RGBA')
    over = Image.new('RGBA', img.size, (0, 0, 0, 0))
    d = ImageDraw.Draw(over)
    d.ellipse((380 * s, -140 * s, 760 * s, 240 * s), fill=light + (46,))
    d.ellipse((-120 * s, 220 * s, 220 * s, 520 * s), fill=light + (30,))
    img = Image.alpha_composite(img, over)
    d = ImageDraw.Draw(img)
    d.text((40 * s, 36 * s), course.upper(), font=font('regular', 15 * s), fill=light + (255,))
    fnt = font('bold', 38 * s)
    y = 100 * s
    for line in wrap(d, lesson, fnt, 520 * s)[:3]:
        d.text((40 * s, y), line, font=fnt, fill=(255, 255, 255, 255))
        y += 50 * s
    # The length sits under the title, clear of the player's controls at the bottom.
    pill = f'{minutes} min'
    f2 = font('regular', 14 * s)
    pw = int(d.textlength(pill, font=f2)) + 24 * s
    top = y + 10 * s
    d.rounded_rectangle((40 * s, top, 40 * s + pw, top + 28 * s), radius=14 * s, fill=light + (255,))
    d.text((52 * s, top + 5 * s), pill, font=f2, fill=dark + (255,))
    img.convert('RGB').resize((1280, 720), Image.LANCZOS).save(path)


def video(path, card, seconds):
    # A still picture at one frame a second costs a few kilobytes per second.
    run('-loop', '1', '-framerate', '1', '-i', card, '-t', str(seconds), '-c:v', 'libvpx', '-crf', '10', '-b:v', '300k',
        '-deadline', 'realtime', '-cpu-used', '8', '-g', '30', '-pix_fmt', 'yuv420p', '-an', path)


def cover(path, colours, motif):
    """An abstract 1600x1000 picture for the gallery."""
    dark, mid, light = map(hexrgb, colours)
    s, (w, h) = 2, (1600, 1000)
    img = gradient((w * s, h * s), mid, dark).convert('RGBA')
    over = Image.new('RGBA', img.size, (0, 0, 0, 0))
    d = ImageDraw.Draw(over)

    def circle(cx, cy, r, fill, width=0):
        box = ((cx - r) * s, (cy - r) * s, (cx + r) * s, (cy + r) * s)
        d.ellipse(box, fill=None if width else fill, outline=fill if width else None, width=width * s)

    if motif == 'lens':
        for i, r in enumerate((430, 340, 250, 160, 80)):
            circle(900, 500, r, light + (60 - i * 8,), width=14)
        circle(900, 500, 70, light + (170,))
        circle(760, 380, 34, (255, 255, 255, 150))
    elif motif == 'nodes':
        pts = [(300, 240), (620, 420), (980, 250), (1260, 560), (820, 740), (420, 700), (1180, 220)]
        for a, b in ((0, 1), (1, 2), (2, 3), (3, 4), (4, 5), (5, 1), (2, 6), (6, 3), (1, 4)):
            d.line((pts[a][0] * s, pts[a][1] * s, pts[b][0] * s, pts[b][1] * s), fill=light + (90,), width=6 * s)
        for i, (x, y) in enumerate(pts):
            circle(x, y, 44 if i % 2 else 30, light + ((210 if i % 2 else 130),))
    elif motif == 'stripes':
        for i in range(-6, 14):
            x = 160 * i
            d.polygon([(x * s, 0), ((x + 70) * s, 0), ((x + 70 + 520) * s, h * s), ((x + 520) * s, h * s)], fill=light + ((34 if i % 2 else 18),))
        for r, a in ((300, 60), (210, 90), (120, 140)):
            circle(1000, 520, r, light + (a,), width=16)
    elif motif == 'staff':
        for i in range(5):
            y = 330 + i * 70
            d.line((120 * s, y * s, 1480 * s, y * s), fill=light + (120,), width=6 * s)
        for x, y in ((300, 540), (560, 400), (820, 470), (1080, 330), (1320, 400)):
            circle(x, y, 40, light + (230,))
            d.line(((x + 36) * s, y * s, (x + 36) * s, (y - 230) * s), fill=light + (230,), width=9 * s)
    elif motif == 'hills':
        circle(1180, 300, 130, (255, 255, 255, 120))
        for i, (base, amp, a) in enumerate(((620, 90, 70), (720, 120, 110), (840, 100, 160))):
            pts = [(0, h)]
            for x in range(0, w + 41, 40):
                pts.append((x, base + amp * math.sin(x / 210.0 + i * 1.7)))
            pts.append((w, h))
            d.polygon([(px * s, py * s) for px, py in pts], fill=light + (a,))
    elif motif == 'bubbles':
        for x, y, bw, bh, a in ((240, 260, 640, 280, 70), (700, 470, 600, 260, 120), (320, 700, 520, 190, 90)):
            d.rounded_rectangle((x * s, y * s, (x + bw) * s, (y + bh) * s), radius=70 * s, fill=light + (a,))
        circle(1220, 300, 120, light + (140,))
    img = Image.alpha_composite(img, over)
    img.convert('RGB').resize((w, h), Image.LANCZOS).save(path, quality=84)


def pdf(path, course, title, lines):
    c = canvas.Canvas(path, pagesize=A4)
    c.setFont('Helvetica-Bold', 26)
    c.drawString(72, 760, title)
    c.setFont('Helvetica', 12)
    c.setFillGray(0.4)
    c.drawString(72, 738, course)
    c.setFillGray(0)
    c.setFont('Helvetica', 14)
    y = 690
    for ln in lines:
        c.drawString(72, y, '•  ' + ln)
        y -= 30
    c.showPage()
    c.save()


def main(root):
    tmp = tempfile.mkdtemp()
    for course in COURSES:
        base = os.path.join(root, course['name'])
        os.makedirs(base, exist_ok=True)
        cover(os.path.join(base, 'cover.jpg'), course['colours'], course['motif'])
        for folder, lessons in course['sections']:
            dest = os.path.join(base, folder) if folder else base
            os.makedirs(dest, exist_ok=True)
            for title, kind in lessons:
                name = title.split(' - ', 1)[1]
                if kind == 'pdf':
                    pdf(os.path.join(dest, title + '.pdf'), course['name'], name,
                        ['Keep this page next to you while you practise', 'Pause the video whenever you need to',
                         'Try each exercise twice before moving on', 'Write down questions as they come up'])
                elif kind == 'md':
                    with open(os.path.join(dest, title + '.md'), 'w') as f:
                        f.write(f'# {name}\n\nTry these on your own, then compare with the video.\n\n'
                                '- [ ] Do the first exercise\n- [ ] Do the second exercise\n- [ ] Write down one thing that surprised you\n\n'
                                'Take your time: **nobody is counting**.\n')
                else:
                    card = os.path.join(tmp, 'card.png')
                    title_card(card, course['name'], course['colours'], name, max(1, round(kind / 60)))
                    video(os.path.join(dest, title + '.webm'), card, kind)
    # a single long talk in the library root, with chapters and a picture
    talk = TALK['name']
    card = os.path.join(tmp, 'talk.png')
    title_card(card, 'Conference talk', TALK['colours'], talk, round(TALK['seconds'] / 60))
    video(os.path.join(root, talk + '.webm'), card, TALK['seconds'])
    with open(os.path.join(root, talk + '.chapters.txt'), 'w') as f:
        f.write(TALK['chapters'])
    cover(os.path.join(root, talk + '.jpg'), TALK['colours'], TALK['motif'])
    print('demo library ready:', root)


if __name__ == '__main__':
    main(sys.argv[1] if len(sys.argv) > 1 else sys.exit(__doc__))
