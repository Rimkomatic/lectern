"""Writes a small sample course library for the browser smoke test.

Usage: python3 make_library.py DIRECTORY
Needs ffmpeg (VP8/Vorbis) and `pip install pillow reportlab`.
"""
import subprocess, sys, zipfile, os
from PIL import Image
from reportlab.pdfgen import canvas

root = sys.argv[1] if len(sys.argv) > 1 else sys.exit(__doc__)
gb = f'{root}/Go Basics'
intro = f'{gb}/01 - Introduction'
data = f'{gb}/02 - Working With Data'
py = f'{root}/Python Tips/Episodes'

for d in (intro, data, py):
    os.makedirs(d, exist_ok=True)


def ff(*args):
    subprocess.run(['ffmpeg', '-y', '-loglevel', 'error', *args], check=True)


def video(path, seconds):
    ff('-f', 'lavfi', '-i', f'testsrc=duration={seconds}:size=320x240:rate=15',
       '-f', 'lavfi', '-i', f'sine=frequency=440:duration={seconds}',
       '-c:v', 'libvpx', '-b:v', '150k', '-deadline', 'realtime', '-cpu-used', '8',
       '-c:a', 'libvorbis', '-shortest', path)


for name in ['01 - Welcome.webm', '02 - Setup.webm']:
    video(f'{intro}/{name}', 40)

ff('-f', 'lavfi', '-i', 'sine=frequency=330:duration=15', '-c:a', 'libvorbis', f'{py}/01 - Tip One.ogg')

with open(f'{intro}/01 - Welcome.en.srt', 'w') as f:
    f.write('1\n00:00:01,000 --> 00:00:03,000\nHello and welcome\n\n2\n00:00:04,000 --> 00:00:06,000\nLet us begin\n')

c = canvas.Canvas(f'{intro}/03 - Slides.pdf')
for i in range(1, 4):
    c.setFont('Helvetica', 36)
    c.drawString(100, 700, f'Slide {i}')
    c.showPage()
c.save()

with open(f'{data}/01 - Notes.md', 'w') as f:
    f.write('# Notes heading\n\nSome **bold** text and a [link](https://example.com).\n\n<script>window.__xss = 1</script>\n')
with open(f'{data}/02 - Sample.csv', 'w') as f:
    f.write('name,score\nAda,10\n"Smith, J",7\n')
Image.new('RGB', (400, 300), (200, 40, 40)).save(f'{data}/03 - Diagram.png')
with open(f'{data}/04 - Further Reading.url', 'w') as f:
    f.write('[InternetShortcut]\nURL=https://example.com/docs\n')
with zipfile.ZipFile(f'{data}/05 - Exercise Files.zip', 'w') as z:
    z.writestr('a.txt', 'hello')

with open(f'{gb}/README.md', 'w') as f:
    f.write('# Go Basics\n\nCourse-level readme.\n')
Image.new('RGB', (300, 200), (30, 90, 200)).save(f'{gb}/cover.jpg')

# a course folder with nothing but videos: no sections
flat = f'{root}/Flat Videos'
os.makedirs(flat, exist_ok=True)
for name in ['01 - First.webm', '02 - Second.webm']:
    video(f'{flat}/{name}', 20)

# a single long video straight in the library root: a course of its own, with
# a chapters file and a picture beside it
video(f'{root}/Long Lecture.webm', 90)
with open(f'{root}/Long Lecture.chapters.txt', 'w') as f:
    f.write('0:00 Opening\n0:30 Middle part\n1:00 Closing\n')
Image.new('RGB', (320, 200), (40, 160, 90)).save(f'{root}/Long Lecture.jpg')
print('library ready:', root)
