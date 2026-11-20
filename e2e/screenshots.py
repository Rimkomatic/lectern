"""Takes the README screenshots from a running server that holds the demo library.

Run it through screenshots.sh (which builds everything, starts a fresh server
and makes the library). It needs E2E_BASE (the server) and SHOTS_OUT (where the
PNG files go); E2E_CHROME may name a Chrome/Chromium binary.

It creates the person "alex", gives them some progress in the courses so the
pictures show the "Continue learning" row, ticks and percentages, and then
photographs: the home page, a course, a lesson in dark mode, and the phone
layout.
"""
import os
import sys
import time

from PIL import Image
from playwright.sync_api import sync_playwright

BASE = os.environ.get('E2E_BASE', 'http://127.0.0.1:18081')
OUT = os.environ.get('SHOTS_OUT', 'docs/screenshots')


def api(page, path, method='GET', body=None):
    r = page.evaluate(
        """async ([p, method, body]) => {
            const r = await fetch('/api/v1' + p, {method, credentials: 'same-origin',
              headers: body ? {'Content-Type': 'application/json'} : undefined,
              body: body ? JSON.stringify(body) : undefined});
            let j = null; try { j = await r.json(); } catch (e) {}
            return {status: r.status, body: j};
        }""", [path, method, body])
    if r['status'] >= 300:
        sys.exit(f'{method} {path} -> {r["status"]} {r["body"]}')
    return r['body']


def wait_for_library(page, titles, timeout=60):
    end = time.time() + timeout
    while time.time() < end:
        got = {c['title'] for c in api(page, '/courses')}
        if set(titles) <= got:
            return
        time.sleep(0.5)
    sys.exit('the library did not show up')


def lessons(page, course_id):
    """Every item of a course in order, with its section."""
    tree = api(page, f'/courses/{course_id}')
    return [it for sec in tree['sections'] for it in sec['items']]


def wait_until(page, js, timeout=20, what=''):
    """Polls a JavaScript function until it returns something true. (Playwright's
    own wait_for_function evaluates a string, which the page's content security
    policy forbids.)"""
    end = time.time() + timeout
    while time.time() < end:
        if page.evaluate(js):
            return
        time.sleep(0.25)
    sys.exit(f'timed out waiting for {what or js}')


def settle(page):
    """The pictures in view are loaded (those scrolled out of sight are lazy) and the entrance animations are over."""
    wait_until(page, """() => Array.from(document.images)
        .filter(i => { const r = i.getBoundingClientRect(); return r.top < innerHeight && r.left < innerWidth && r.right > 0; })
        .every(i => i.complete && i.naturalWidth > 0)""", what='pictures')
    page.wait_for_timeout(1100)


def video_ready(page):
    """The first frame is on screen and the browser's "loading" spinner has gone."""
    page.wait_for_selector('video')
    wait_until(page, "() => document.querySelector('video').readyState >= 4", what='the video')
    page.wait_for_timeout(800)


def shot(page, name):
    os.makedirs(OUT, exist_ok=True)
    path = os.path.join(OUT, name)
    page.screenshot(path=path)
    # Same pixels, about a quarter smaller: these files stay in git for good.
    Image.open(path).convert('RGB').save(path, 'PNG', optimize=True, compress_level=9)
    print('wrote', path)


def main():
    with sync_playwright() as pw:
        launch = dict(headless=True, args=['--no-sandbox', '--mute-audio'])
        if os.environ.get('E2E_CHROME'):
            launch['executable_path'] = os.environ['E2E_CHROME']
        browser = pw.chromium.launch(**launch)

        def context(scheme='light', **kw):
            return browser.new_context(color_scheme=scheme, **kw)

        # --- the person, and their progress -------------------------------------
        ctx = context(viewport={'width': 1280, 'height': 800})
        page = ctx.new_page()
        page.goto(BASE + '/')
        page.wait_for_selector('text=Create the administrator account', timeout=10000)
        page.fill('input[name=username]', 'alex')
        page.fill('input[name=display_name]', 'Alex Rivera')
        page.fill('input[name=password]', 'demo-password-1')
        page.fill('input[name=confirm]', 'demo-password-1')
        page.click('button:has-text("Create account")')
        page.wait_for_selector('h1:has-text("Courses")', timeout=10000)

        wait_for_library(page, ['Photography Basics', 'Python for Beginners', 'Knife Skills',
                                'Music Theory 101', 'Watercolor Landscapes', 'Designing for Humans'])
        courses = {c['title']: c['id'] for c in api(page, '/courses')}

        def post(item_id, body):
            api(page, f'/items/{item_id}/progress', 'POST', {**body, 'client_ts': int(time.time() * 1000)})

        def done(item):
            post(item['id'], {'event': 'done', 'done': True})

        def watched(item, fraction):
            d = item.get('duration') or 300
            post(item['id'], {'event': 'pause', 'position': round(d * fraction), 'duration': d})

        # Oldest first: the most recently touched course comes first in "Continue learning".
        L = {name: lessons(page, cid) for name, cid in courses.items()}
        watched(L['Music Theory 101'][0], 0.6)
        for it in L['Knife Skills'][:1]:
            done(it)
        watched(L['Knife Skills'][1], 0.35)
        talk = L['Designing for Humans'][0]
        api(page, f'/items/{talk["id"]}/chapters', 'POST', {'done': [0, 200]})  # the first two chapters
        watched(talk, 0.43)
        for it in L['Python for Beginners'][:2]:
            done(it)
        watched(L['Python for Beginners'][3], 0.3)
        for it in L['Photography Basics'][:4]:
            done(it)
        watched(L['Photography Basics'][4], 0.45)
        photo_resume = L['Photography Basics'][4]

        # --- 1. home --------------------------------------------------------------
        page.goto(BASE + '/#/')
        page.reload()  # the lists on screen were loaded before the progress existed
        page.wait_for_selector('ul[aria-label="All courses"] li')
        settle(page)
        shot(page, 'home.png')

        # --- 2. a course ------------------------------------------------------------
        page.goto(BASE + f'/#/course/{courses["Photography Basics"]}')
        page.wait_for_selector('h1:has-text("Photography Basics")')
        settle(page)
        shot(page, 'course.png')
        ctx.close()

        # --- 3. a lesson, dark -------------------------------------------------------
        ctx = context('dark', viewport={'width': 1280, 'height': 800})
        page = ctx.new_page()
        page.goto(BASE + '/')
        page.wait_for_selector('input[name=username]')
        page.fill('input[name=username]', 'alex')
        page.fill('input[name=password]', 'demo-password-1')
        page.click('button:has-text("Sign in")')
        page.wait_for_selector('h1:has-text("Courses")')
        page.goto(BASE + f'/#/course/{courses["Photography Basics"]}/item/{photo_resume["id"]}')
        video_ready(page)
        page.wait_for_selector('section[aria-label="Resume"]')
        settle(page)
        shot(page, 'lesson-dark.png')
        ctx.close()

        # --- 4. the phone -------------------------------------------------------------
        ctx = context(viewport={'width': 390, 'height': 844}, device_scale_factor=2, is_mobile=True, has_touch=True)
        page = ctx.new_page()
        page.goto(BASE + '/')
        page.wait_for_selector('input[name=username]')
        page.fill('input[name=username]', 'alex')
        page.fill('input[name=password]', 'demo-password-1')
        page.click('button:has-text("Sign in")')
        page.wait_for_selector('h1:has-text("Courses")')
        page.wait_for_selector('ul[aria-label="All courses"] li')
        settle(page)
        shot(page, 'phone-home.png')
        # The talk with chapters: it fills the small screen and shows the chapter list.
        page.goto(BASE + f'/#/course/{courses["Designing for Humans"]}/item/{talk["id"]}')
        video_ready(page)
        page.wait_for_selector('h2#chapters-h')
        settle(page)
        shot(page, 'phone-lesson.png')
        ctx.close()
        browser.close()


if __name__ == '__main__':
    main()
