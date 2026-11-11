"""Browser smoke test: drives the real UI of a running server with headless Chrome.

Run it through run.sh, which builds everything, starts a server on an emptied
test database and generates the sample library. It expects a FRESH server (it
goes through the first-run setup page), the library from make_library.py, and
E2E_BASE pointing at the server. E2E_CHROME may name a Chrome/Chromium binary;
otherwise Playwright's own browser is used (`playwright install chromium`).
"""
import os, re, sys, time, json, traceback
from playwright.sync_api import sync_playwright

BASE = os.environ.get('E2E_BASE', 'http://127.0.0.1:18080')
results = []          # (ok, name, detail)
console_errors = []

def check(name, cond, detail=''):
    results.append((bool(cond), name, detail))
    print(('PASS ' if cond else 'FAIL ') + name + ((' :: ' + str(detail)) if (detail and not cond) else ''))
    sys.stdout.flush()

def api(page, path, method='GET', body=None):
    return page.evaluate(
        """async ([p, method, body]) => {
            const r = await fetch('/api/v1' + p, {method, credentials: 'same-origin',
              headers: body ? {'Content-Type': 'application/json'} : undefined,
              body: body ? JSON.stringify(body) : undefined});
            let j = null; try { j = await r.json(); } catch (e) {}
            return {status: r.status, body: j};
        }""", [path, method, body])

def wait_until(fn, timeout=10, interval=0.25, desc=''):
    end = time.time() + timeout
    last = None
    while time.time() < end:
        try:
            last = fn()
            if last:
                return last
        except Exception as e:  # keep polling
            last = e
        time.sleep(interval)
    raise AssertionError(f'timeout waiting for {desc}: last={last!r}')

def item_id(page):
    m = re.search(r'/item/(\d+)', page.evaluate('location.hash'))
    return int(m.group(1)) if m else None

def progress(page, iid):
    return api(page, f'/items/{iid}')['body']['item']['progress']

def goto_hash(page, h):
    page.evaluate('h => { location.hash = h }', h)

def step(name):
    def deco(fn):
        print(f'--- {name}')
        try:
            fn()
        except Exception as e:
            check(f'{name}: no exception', False, ''.join(traceback.format_exception_only(type(e), e)).strip()[:400])
            traceback.print_exc(limit=3)
        return fn
    return deco

with sync_playwright() as pw:
    launch = dict(headless=True, args=['--no-sandbox', '--autoplay-policy=no-user-gesture-required', '--mute-audio'])
    if os.environ.get('E2E_CHROME'):
        launch['executable_path'] = os.environ['E2E_CHROME']
    browser = pw.chromium.launch(**launch)
    ctx = browser.new_context(viewport={'width': 1100, 'height': 900})
    # external links must not leave the sandbox
    ctx.route('https://example.com/**', lambda r: r.fulfill(status=200, content_type='text/html', body='<h1>example</h1>'))
    page = ctx.new_page()

    def on_console(m):
        if m.type == 'error':
            url = (m.location or {}).get('url', '')
            console_errors.append(f'{m.text} [{url}]')
    page.on('console', on_console)
    page.on('pageerror', lambda e: console_errors.append(f'pageerror: {e}'))

    ids = {}

    @step('first-run setup wizard')
    def _():
        page.goto(BASE + '/')
        page.wait_for_selector('text=Create the administrator account', timeout=8000)
        check('setup page shown on a fresh server', True)
        body = page.inner_text('body')
        check('setup page has no helper text', page.locator('small, .hint').count() == 0 and not re.search(r'3.32 letters|no users yet|first account', body, re.I), body)
        page.fill('input[name=username]', 'admin')
        page.fill('input[name=display_name]', 'Admin')
        page.fill('input[name=password]', 'adminpass1')
        page.fill('input[name=confirm]', 'adminpass1')
        page.click('button:has-text("Create account")')
        page.wait_for_selector('h1:has-text("Courses")', timeout=8000)
        check('setup signs the admin in and shows the course list', True)
        check('admin link is visible to the admin', page.locator('nav a[href="#/admin"]').count() == 1)

    @step('home page')
    def _():
        page.wait_for_selector('ul[aria-label="All courses"] li', timeout=8000)
        titles = page.locator('ul[aria-label="All courses"] li strong').all_inner_texts()
        check('every course is listed: folders, and a video lying in the library root', sorted(titles) == ['Flat Videos', 'Go Basics', 'Long Lecture', 'Python Tips'], titles)
        img = page.locator('ul[aria-label="All courses"] img')
        check('two courses have a cover image (a folder picture, and a picture beside the lone video)', img.count() == 2, img.count())
        try:
            page.wait_for_function('() => { const i = document.querySelector("ul[aria-label=\\"All courses\\"] img"); return i && i.complete && i.naturalWidth > 0 }', timeout=8000)
            check('cover image loads', True)
        except Exception as e:
            check('cover image loads', False, e)
        # the other course gets a drawn cover (initials), not a broken image
        check('a course without a cover gets drawn artwork with its initials', 'PT' in page.locator('ul[aria-label="All courses"] li', has_text='Python Tips').inner_text())
        courses = api(page, '/courses')['body']
        for c in courses:
            ids[c['title']] = c['id']
            tree = api(page, f"/courses/{c['id']}")['body']
            for sec in tree['sections']:
                for it in sec['items']:
                    ids[it['name']] = it['id']
        print('   ids:', json.dumps(ids))
        # search
        page.fill('input[type=search]', 'python')
        page.wait_for_function('() => document.querySelectorAll("ul[aria-label=\\"All courses\\"] li").length === 1', timeout=5000)
        check('search narrows the list', page.locator('ul[aria-label="All courses"] li strong').inner_text() == 'Python Tips')
        page.fill('input[type=search]', 'zzzz')
        page.wait_for_selector('text=No courses match', timeout=5000)
        check('search shows an empty state', True)
        page.fill('input[type=search]', '')
        page.wait_for_selector('ul[aria-label="All courses"] li >> nth=1', timeout=5000)

    gb = lambda: ids['Go Basics']

    @step('course page outline')
    def _():
        page.locator('ul[aria-label="All courses"] a', has_text='Go Basics').click()
        page.wait_for_selector('h1:has-text("Go Basics")', timeout=5000)
        heads = page.locator('main h3').all_inner_texts()
        check('sections in order with clean names', heads == ['Course files', 'Introduction', 'Working With Data'], heads)
        check('"Start course" shown for an unopened course', page.locator('a', has_text='Start course').count() == 1)
        check('subtitle file is not listed as an item', page.locator('li', has_text='.srt').count() == 0 and page.locator('li', has_text='Welcome').count() == 1)
        check('video duration is shown (probed by the scanner)', '40s' in page.locator('li', has_text='Welcome').inner_text() or '1m' in page.locator('li', has_text='Welcome').inner_text(), page.locator('li', has_text='Welcome').inner_text())
        check('link items have no download', page.locator('li', has_text='Further Reading').locator('a', has_text='Download').count() == 0)
        check('other items have a download link', page.locator('li', has_text='Exercise Files').locator('a', has_text='Download').count() == 1)

    @step('video: play, progress, resume')
    def _():
        page.locator('a[href^="#/course/"]', has_text='Welcome').first.click()
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
        dur = page.evaluate('() => document.querySelector("video").duration')
        check('video metadata loads and duration is ~40s', 39 < dur < 41, dur)
        check('subtitle track is attached', page.evaluate('() => document.querySelector("video").textTracks.length') == 1)
        iid = item_id(page)
        check('opening the item creates progress', wait_until(lambda: progress(page, iid), 5, desc='progress after open') is not None)
        page.evaluate('() => { const v = document.querySelector("video"); v.currentTime = 12; v.play(); }')
        time.sleep(3.2)
        page.evaluate('() => document.querySelector("video").pause()')
        time.sleep(1.2)
        p = progress(page, iid)
        check('pause saves the position', p and 12 <= p['position'] <= 20 and p['state'] == 'started', p)
        check('duration is stored from the player', p and p['duration'] and 39 < p['duration'] < 41, p)
        saved = p['position']
        goto_hash(page, '#/')
        page.wait_for_selector('section[aria-label="Continue learning"]', timeout=8000)
        check('home shows "Continue learning"', page.locator('section[aria-label="Continue learning"]', has_text='Go Basics').count() == 1)
        href = page.locator('section[aria-label="Continue learning"] a').first.get_attribute('href')
        check('continue link carries the time', re.search(r't=\d+', href) is not None, href)
        page.locator('section[aria-label="Continue learning"] a').first.click()
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 && v.currentTime > 5 }', timeout=15000)
        ct = page.evaluate('() => document.querySelector("video").currentTime')
        check('continue opens at the saved position', abs(ct - saved) < 2, (ct, saved))
        check('no resume prompt after an explicit continue', page.locator('section[aria-label="Resume"]').count() == 0)

    @step('video: resume prompt (ask mode)')
    def _():
        goto_hash(page, f'#/course/{gb()}')
        page.wait_for_selector('section[aria-label="Resume"]', timeout=8000)
        txt = page.locator('section[aria-label="Resume"]').inner_text()
        check('course page asks whether to continue', 'Continue where you left off?' in txt and 'Welcome' in txt, txt)
        check('course page offers start-from-beginning', page.locator('section[aria-label="Resume"] a', has_text='Start from the beginning').count() == 1)
        page.locator('a[href^="#/course/"]', has_text='Welcome').first.click()
        page.wait_for_selector('section[aria-label="Resume"] button:has-text("Resume")', timeout=8000)
        check('item page shows the resume prompt', 'Continue from 0:' in page.locator('section[aria-label="Resume"]').inner_text())
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
        check('player waits at 0 while asking', page.evaluate('() => document.querySelector("video").currentTime') < 1)
        page.click('button:has-text("Resume")')
        page.wait_for_function('() => document.querySelector("video").currentTime > 5', timeout=5000)
        check('Resume jumps to the saved spot and hides the prompt', page.locator('section[aria-label="Resume"]').count() == 0)
        # start over
        goto_hash(page, f'#/course/{gb()}')
        page.locator('a[href^="#/course/"]', has_text='Welcome').first.click()
        page.wait_for_selector('button:has-text("Start over")', timeout=8000)
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
        page.click('button:has-text("Start over")')
        time.sleep(1.2)
        iid = item_id(page)
        p = progress(page, iid)
        check('Start over resets the saved position', p['position'] < 1, p)

    @step('video: autoplay next and the "not done" hold')
    def _():
        goto_hash(page, f"#/course/{gb()}/item/{ids['01 - Welcome.webm']}?t=36")
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 && v.currentTime > 30 }', timeout=15000)
        page.evaluate('() => document.querySelector("video").play()')
        page.wait_for_selector('[role=status]:has-text("Playing")', timeout=12000)
        check('countdown appears when the video ends', True)
        iid = ids['01 - Welcome.webm']
        wait_until(lambda: progress(page, iid)['state'] == 'done', 6, desc='done after end')
        check('finishing a video marks it done', progress(page, iid)['state'] == 'done')
        wait_until(lambda: page.locator('#course-outline li[data-status="done"]', has_text='Welcome').count() == 1, 5, desc='sidebar marks it done')
        check('sidebar shows it done', True)
        # cancel -> stay; then mark not done and leave
        page.click('button:has-text("Cancel")')
        time.sleep(0.5)
        check('Cancel stops the countdown', page.locator('[role=status]:has-text("Playing")').count() == 0)
        page.locator('p button:has-text("Mark not done")').click()
        wait_until(lambda: progress(page, iid)['state'] == 'started', 5, desc='not done')
        goto_hash(page, '#/')
        time.sleep(1.5)
        check('leaving the page does not silently re-complete it', progress(page, iid)['state'] == 'started', progress(page, iid))
        # autoplay into next
        goto_hash(page, f"#/course/{gb()}/item/{iid}?t=37")
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 && v.currentTime > 30 }', timeout=15000)
        page.evaluate('() => document.querySelector("video").play()')
        nxt = ids['02 - Setup.webm']
        wait_until(lambda: item_id(page) == nxt, 15, desc='navigate to next item')
        check('autoplay moves on to the next item', item_id(page) == nxt)
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 && !v.paused }', timeout=10000)
        check('the next video starts playing by itself', True)
        page.evaluate('() => document.querySelector("video").pause()')

    @step('pdf viewer')
    def _():
        goto_hash(page, f"#/course/{gb()}/item/{ids['03 - Slides.pdf']}")
        page.wait_for_selector('button:has-text("Next page")', timeout=15000)
        page.wait_for_function('() => { const c = document.querySelector("canvas"); return c && c.width > 100 && !c.hidden }', timeout=15000)
        check('pdf page renders on a canvas', True)
        check('page count is shown', 'of 3' in page.locator('label', has_text='Page').inner_text(), page.locator('label', has_text='Page').inner_text())
        iid = ids['03 - Slides.pdf']
        page.click('button:has-text("Next page")')
        time.sleep(1.0)
        p = progress(page, iid)
        check('turning a page saves it', p['page'] == 2 and p['pages'] == 3 and p['state'] == 'started', p)
        page.click('button:has-text("Next page")')
        wait_until(lambda: progress(page, iid)['state'] == 'done', 5, desc='pdf done on last page')
        check('reaching the last page completes the pdf', True)
        check('next is disabled on the last page', page.locator('button:has-text("Next page")').is_disabled())
        # zoom
        before = page.evaluate('() => document.querySelector("canvas").width')
        page.click('button[aria-label="Zoom in"]')
        time.sleep(0.8)
        after = page.evaluate('() => document.querySelector("canvas").width')
        check('zoom enlarges the page', after > before, (before, after))
        # resume offer for a pdf: mark not done (keeps page 3), reopen
        page.locator('p button:has-text("Mark not done")').click()
        wait_until(lambda: progress(page, iid)['state'] == 'started', 5, desc='pdf not done')
        goto_hash(page, '#/')
        time.sleep(0.8)
        goto_hash(page, f"#/course/{gb()}/item/{iid}")
        page.wait_for_selector('section[aria-label="Resume"]:has-text("page 3")', timeout=10000)
        check('pdf offers to continue from the saved page', True)
        # clicked at once, i.e. possibly before pdf.js has finished loading the file
        page.click('section[aria-label="Resume"] button:has-text("Resume")')
        page.wait_for_function('() => document.querySelector("input[type=number]").value === "3"', timeout=10000)
        check('Resume jumps to the saved page', True)
        time.sleep(1.0)
        check('Resume does not overwrite the saved page', progress(page, iid)['page'] == 3, progress(page, iid))

    @step('text, csv, image, link, other')
    def _():
        # markdown
        goto_hash(page, f"#/course/{gb()}/item/{ids['01 - Notes.md']}")
        page.wait_for_selector('article h1', timeout=8000)
        check('markdown renders', page.locator('article h1').inner_text() == 'Notes heading' and page.locator('article strong').count() == 1)
        check('markdown scripts are stripped', page.evaluate('() => window.__xss === undefined') and page.locator('article script').count() == 0)
        a = page.locator('article a')
        check('markdown links open safely', a.get_attribute('target') == '_blank' and 'noopener' in (a.get_attribute('rel') or ''))
        iid = ids['01 - Notes.md']
        wait_until(lambda: progress(page, iid) and progress(page, iid)['state'] == 'done', 5, desc='md done on open')
        check('opening notes marks them done (done_on_open)', True)
        # csv
        goto_hash(page, f"#/course/{gb()}/item/{ids['02 - Sample.csv']}")
        page.wait_for_selector('table th', timeout=8000)
        ths = page.locator('table th').all_inner_texts()
        tds = page.locator('table td').all_inner_texts()
        check('csv header and quoted cell', ths == ['name', 'score'] and tds == ['Ada', '10', 'Smith, J', '7'], (ths, tds))
        # image
        goto_hash(page, f"#/course/{gb()}/item/{ids['03 - Diagram.png']}")
        page.wait_for_function('() => { const i = document.querySelector("main img"); return i && i.complete && i.naturalWidth === 400 }', timeout=8000)
        check('image loads', True)
        wait_until(lambda: progress(page, ids['03 - Diagram.png']) and progress(page, ids['03 - Diagram.png'])['state'] == 'done', 5, desc='image done')
        check('viewing an image marks it done', True)
        # link
        goto_hash(page, f"#/course/{gb()}/item/{ids['04 - Further Reading.url']}")
        page.wait_for_selector('a:has-text("Open example.com")', timeout=8000)
        a = page.locator('a:has-text("Open example.com")')
        check('link opens in a new tab safely', a.get_attribute('target') == '_blank' and 'noopener' in (a.get_attribute('rel') or '') and a.get_attribute('href') == 'https://example.com/docs')
        iid = ids['04 - Further Reading.url']
        p0 = progress(page, iid)
        check('link is not done before it is clicked', not p0 or p0['state'] != 'done', p0)
        with ctx.expect_page() as popup:
            a.click()
        popup.value.close()
        wait_until(lambda: progress(page, iid) and progress(page, iid)['state'] == 'done', 5, desc='link done on click')
        check('clicking the link marks it done', True)
        # other (zip)
        goto_hash(page, f"#/course/{gb()}/item/{ids['05 - Exercise Files.zip']}")
        page.wait_for_selector("text=can't be previewed", timeout=8000)
        href = page.locator('main a:has-text("Download")').first.get_attribute('href')
        r = page.request.get(BASE + href)
        check('download works with attachment disposition', r.status == 200 and 'attachment' in r.headers.get('content-disposition', '') and r.body()[:2] == b'PK', (r.status, r.headers.get('content-disposition')))
        # text-like files served safely
        r = page.request.get(BASE + f"/api/v1/items/{ids['01 - Notes.md']}/stream")
        check('markdown is served as text/plain with a sandbox CSP', r.headers.get('content-type', '').startswith('text/plain') and 'sandbox' in r.headers.get('content-security-policy', ''), r.headers)
        # range request on video
        r = page.request.get(BASE + f"/api/v1/items/{ids['01 - Welcome.webm']}/stream", headers={'Range': 'bytes=0-99'})
        check('video stream supports Range', r.status == 206 and len(r.body()) == 100, r.status)

    @step('home gallery: filters and clearing the search')
    def _():
        goto_hash(page, '#/')
        page.wait_for_selector('ul[aria-label="All courses"] li', timeout=8000)
        group = '[role=group][aria-label="Filter courses"] '
        titles = lambda: page.locator('ul[aria-label="All courses"] li strong').all_inner_texts()
        page.click(group + 'button:has-text("In progress")')
        wait_until(lambda: titles() == ['Go Basics'], 4, desc='in-progress filter')
        check('"In progress" shows only started courses', titles() == ['Go Basics'], titles())
        check('the chosen filter is announced as pressed', page.locator(group + 'button:has-text("In progress")').get_attribute('aria-pressed') == 'true')
        page.click(group + 'button:has-text("Not started")')
        untouched = ['Flat Videos', 'Long Lecture', 'Python Tips']
        wait_until(lambda: sorted(titles()) == untouched, 4, desc='not-started filter')
        check('"Not started" shows only untouched courses', sorted(titles()) == untouched, titles())
        page.click(group + 'button:has-text("Completed")')
        page.wait_for_selector('text=Nothing in this view', timeout=4000)
        check('a filter with no courses says so', True)
        page.click('button:has-text("Show all courses")')
        wait_until(lambda: sorted(titles()) == ['Flat Videos', 'Go Basics', 'Long Lecture', 'Python Tips'], 4, desc='all courses again')
        check('"Show all courses" resets the filter', True)
        page.fill('input[type=search]', 'python')
        wait_until(lambda: titles() == ['Python Tips'], 4, desc='search narrows')
        page.click('button:has-text("Clear the search text")')
        wait_until(lambda: sorted(titles()) == ['Flat Videos', 'Go Basics', 'Long Lecture', 'Python Tips'], 4, desc='search cleared')
        check('the clear button empties the search and keeps the cursor in it',
              page.input_value('input[type=search]') == '' and page.evaluate('() => document.activeElement && document.activeElement.type') == 'search')
        check('the clear button is gone when the field is empty', page.locator('button:has-text("Clear the search text")').count() == 0)

    @step('audio')
    def _():
        goto_hash(page, f"#/course/{ids['Python Tips']}/item/{ids['01 - Tip One.ogg']}")
        page.wait_for_function('() => { const a = document.querySelector("audio"); return a && a.readyState >= 1 }', timeout=10000)
        check('audio items use an audio element', page.locator('video').count() == 0)

    @step('lesson sidebar')
    def _():
        goto_hash(page, f"#/course/{gb()}/item/{ids['02 - Setup.webm']}")
        side = page.locator('#course-outline')
        toggle = page.locator('button[aria-controls="course-outline"]')
        side.wait_for(state='visible', timeout=8000)
        page.wait_for_function('() => document.querySelectorAll("#course-outline li").length > 0', timeout=8000)
        n_items = sum(len(sec['items']) for sec in api(page, f'/courses/{gb()}')['body']['sections'])
        check('sidebar lists every item of the course', side.locator('li').count() == n_items, (side.locator('li').count(), n_items))
        check('sidebar sits to the left of the lesson', page.evaluate('() => document.getElementById("course-outline").getBoundingClientRect().right <= document.getElementById("main").getBoundingClientRect().left + 1'))
        cur = side.locator('a[aria-current="true"]')
        check('the lesson on screen is the highlighted one', cur.count() == 1 and 'Setup' in cur.inner_text(), cur.all_inner_texts())
        check('the highlighted lesson is in view (its section is open)', cur.is_visible())
        side.locator('a', has_text='Welcome').click()
        wait_until(lambda: item_id(page) == ids['01 - Welcome.webm'], 8, desc='sidebar link navigates')
        wait_until(lambda: side.locator('a[aria-current="true"]').count() == 1 and 'Welcome' in side.locator('a[aria-current="true"]').inner_text(), 8, desc='highlight follows')
        check('clicking a lesson in the sidebar opens it and moves the highlight', True)
        check('the sidebar stays open between lessons', side.is_visible())
        # sections fold and unfold
        sec = side.locator('button[aria-controls^="sec-"]').first
        before = sec.get_attribute('aria-expanded')
        sec.click()
        check('a section header folds and unfolds', sec.get_attribute('aria-expanded') != before)
        sec.click()
        # collapsing
        open_w = page.evaluate('() => document.getElementById("main").getBoundingClientRect().width')
        check('toggle reports the sidebar as open', toggle.get_attribute('aria-expanded') == 'true')
        toggle.click()
        wait_until(lambda: not side.is_visible(), 4, desc='sidebar hides')
        check('collapsing hides the sidebar', toggle.get_attribute('aria-expanded') == 'false')
        wait_until(lambda: page.evaluate('() => document.getElementById("main").getBoundingClientRect().width') > open_w + 100, 4, desc='lesson widens')
        check('the lesson takes the freed width', True)
        check('hidden sidebar is out of the tab order', page.evaluate('() => document.getElementById("course-outline").inert') is True)
        check('the choice is remembered on this device', page.evaluate('() => localStorage.getItem("lectern-sidebar")') == '0')
        page.reload()
        toggle.wait_for(timeout=8000)
        check('collapsed state survives a reload', toggle.get_attribute('aria-expanded') == 'false' and not side.is_visible())
        # another lesson page keeps it collapsed too
        goto_hash(page, f"#/course/{gb()}/item/{ids['02 - Setup.webm']}")
        toggle.wait_for(timeout=8000)
        check('collapsed state carries over to the next lesson', toggle.get_attribute('aria-expanded') == 'false' and not side.is_visible())
        toggle.click()
        side.wait_for(state='visible', timeout=4000)
        check('expanding brings the sidebar back', toggle.get_attribute('aria-expanded') == 'true' and page.evaluate('() => localStorage.getItem("lectern-sidebar")') == '1')

    @step('light and dark theme')
    def _():
        theme = lambda: page.evaluate('() => document.documentElement.dataset.theme')
        stored = lambda: page.evaluate('() => localStorage.getItem("lectern-theme")')
        page_bg = lambda: page.evaluate('() => getComputedStyle(document.body).backgroundColor')
        goto_hash(page, '#/')
        page.wait_for_selector('h1:has-text("Courses")', timeout=5000)
        page.emulate_media(color_scheme='light')
        wait_until(lambda: theme() == 'light', 3, desc='system light')
        check('with no saved choice the theme follows the system (light)', stored() is None and theme() == 'light', (stored(), theme()))
        check('the light palette is applied', page_bg() == 'rgb(246, 244, 239)', page_bg())
        page.get_by_role('button', name='Switch to dark mode').click()
        wait_until(lambda: theme() == 'dark', 3, desc='dark after the quick switch')
        wait_until(lambda: page_bg() == 'rgb(13, 15, 18)', 3, desc='dark palette')
        check('the top-bar switch turns dark mode on and remembers it', stored() == 'dark', stored())
        page.reload()
        page.wait_for_selector('h1:has-text("Courses")', timeout=8000)
        check('dark mode survives a reload', theme() == 'dark' and page_bg() == 'rgb(13, 15, 18)', (theme(), page_bg()))
        # the saved choice is applied before the app code runs (no flash of the wrong theme)
        early = page.evaluate("""async () => {
            const html = await (await fetch('/')).text();
            return {init: /theme-init\\.js/.test(html), inline: /<script(?![^>]*\\bsrc=)[^>]*>[^<]/.test(html)};
        }""")
        check('theme is set by an external script (no inline script, so the CSP stays strict)', early['init'] and not early['inline'], early)
        # Settings: the three-way choice
        goto_hash(page, '#/settings')
        page.wait_for_selector('h1:has-text("Settings")', timeout=5000)
        check('settings shows the saved choice', page.locator('input[name=theme][value=dark]').is_checked())
        page.locator('label.theme-opt', has_text='Light').click()
        wait_until(lambda: theme() == 'light', 3, desc='light from settings')
        check('picking Light in settings applies and saves it', stored() == 'light', stored())
        page.locator('label.theme-opt', has_text='System').click()
        wait_until(lambda: stored() is None, 3, desc='system clears the saved choice')
        page.emulate_media(color_scheme='dark')
        wait_until(lambda: theme() == 'dark', 3, desc='system dark')
        check('System follows the device setting live', theme() == 'dark' and stored() is None, (theme(), stored()))
        page.emulate_media(color_scheme='light')
        wait_until(lambda: theme() == 'light', 3, desc='system light again')
        check('and follows it back', True)
        check('the colour scheme hint for form controls and scrollbars matches', page.evaluate('() => getComputedStyle(document.documentElement).colorScheme') == 'light')

    @step('phone layout and the course-content drawer')
    def _():
        ctx3 = browser.new_context(viewport={'width': 390, 'height': 844}, is_mobile=True, has_touch=True)
        p3 = ctx3.new_page()
        p3.on('console', on_console)
        p3.on('pageerror', lambda e: console_errors.append(f'pageerror: {e}'))
        p3.goto(BASE + '/')
        p3.wait_for_selector('h2:has-text("Sign in")', timeout=8000)
        p3.fill('input[name=username]', 'admin')
        p3.fill('input[name=password]', 'adminpass1')
        p3.click('button:has-text("Sign in")')
        p3.wait_for_selector('h1:has-text("Courses")', timeout=8000)
        no_scroll = lambda: p3.evaluate('() => document.documentElement.scrollWidth <= window.innerWidth + 1')
        check('home does not scroll sideways on a phone', no_scroll())
        goto_hash(p3, f'#/course/{gb()}')
        p3.wait_for_selector('h1:has-text("Go Basics")', timeout=8000)
        check('course page does not scroll sideways on a phone', no_scroll())
        goto_hash(p3, f"#/course/{gb()}/item/{ids['02 - Setup.webm']}")
        side = p3.locator('#course-outline')
        toggle = p3.locator('button[aria-controls="course-outline"]')
        toggle.wait_for(timeout=8000)
        check('on a phone the sidebar starts closed', toggle.get_attribute('aria-expanded') == 'false' and not side.is_visible())
        check('lesson page does not scroll sideways on a phone', no_scroll())
        toggle.click()
        side.wait_for(state='visible', timeout=4000)
        check('the toggle opens the drawer', toggle.get_attribute('aria-expanded') == 'true')
        check('the lesson behind the drawer is inert', p3.evaluate('() => document.getElementById("main").inert') is True)
        check('the drawer shows the highlighted lesson', 'Setup' in side.locator('a[aria-current="true"]').inner_text())
        # the player's keys must not act on a lesson hidden behind the drawer
        p3.evaluate('() => { const v = document.querySelector("video"); v.pause(); v.volume = 0.5; }')
        p3.keyboard.press('ArrowUp')
        p3.keyboard.press('ArrowRight')
        time.sleep(0.4)
        behind = p3.evaluate('() => { const v = document.querySelector("video"); return {vol: v.volume, t: v.currentTime, paused: v.paused} }')
        check('with the drawer open the player keys leave the lesson behind it alone', abs(behind['vol'] - 0.5) < 0.001 and behind['t'] < 1 and behind['paused'], behind)
        p3.keyboard.press('Escape')
        wait_until(lambda: not side.is_visible(), 4, desc='Escape closes the drawer')
        check('Escape closes the drawer and gives focus back to the button', p3.evaluate('() => document.activeElement === document.querySelector("button[aria-controls=course-outline]")'))
        check('the lesson is usable again', p3.evaluate('() => document.getElementById("main").inert') is False)
        toggle.click()
        side.wait_for(state='visible', timeout=4000)
        p3.locator('button[aria-label="Close course content"]').click(position={'x': 370, 'y': 400}, force=True)
        wait_until(lambda: not side.is_visible(), 4, desc='scrim click closes the drawer')
        check('tapping outside the drawer closes it', True)
        toggle.click()
        side.wait_for(state='visible', timeout=4000)
        side.locator('a', has_text='Welcome').click()
        wait_until(lambda: item_id(p3) == ids['01 - Welcome.webm'], 8, desc='drawer link navigates')
        p3.locator('button[aria-controls="course-outline"]').wait_for(timeout=8000)
        check('choosing a lesson in the drawer opens it and closes the drawer', not p3.locator('#course-outline').is_visible())
        fits = '() => { const v = document.querySelector("video"); return !!v && v.getBoundingClientRect().right <= window.innerWidth + 1 }'
        try:
            wait_until(lambda: p3.evaluate(fits), 6, desc='the new lesson to show its video')  # the page may still be loading
        except AssertionError:
            pass
        check('the video fits the phone screen', p3.evaluate(fits))
        ctx3.close()

    @step('settings')
    def _():
        goto_hash(page, '#/settings')
        page.wait_for_selector('h1:has-text("Settings")', timeout=5000)
        page.check('input[name=resume][value=auto]')
        page.fill('input[type=number]', '1.5')
        page.click('section:has(h2:has-text("Playback")) button[type=submit]')
        page.wait_for_selector('[role=status]:has-text("Saved")', timeout=5000)
        page.reload()
        page.wait_for_selector('h1:has-text("Settings")', timeout=8000)
        check('settings persist across a reload', page.locator('input[name=resume][value=auto]').is_checked() and page.locator('input[type=number]').input_value() == '1.5')
        s = api(page, '/me/settings') if False else api(page, '/me')['body']['settings']
        check('server stores the settings', s['resume_mode'] == 'auto' and s['playback_rate'] == 1.5, s)
        # auto-resume
        goto_hash(page, f'#/course/{gb()}')
        page.wait_for_selector('section[aria-label="Resume"]', timeout=8000)
        check('auto mode shows a plain continue (no question)', 'Continue where you left off?' not in page.locator('section[aria-label="Resume"]').inner_text())
        # playback rate applies
        goto_hash(page, f"#/course/{gb()}/item/{ids['02 - Setup.webm']}")
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
        check('saved playback speed is applied', abs(page.evaluate('() => document.querySelector("video").playbackRate') - 1.5) < 0.01)
        # bad rate is rejected client-side
        goto_hash(page, '#/settings')
        page.fill('input[type=number]', '9')
        page.click('section:has(h2:has-text("Playback")) button[type=submit]')
        time.sleep(0.5)
        check('out-of-range speed is blocked by the form', page.evaluate('() => document.querySelector("input[type=number]").validity.rangeOverflow') and page.locator('[role=status]:has-text("Saved")').count() == 0)
        check('server still has the old speed', api(page, '/me')['body']['settings']['playback_rate'] == 1.5)
        # back to ask for later steps
        page.fill('input[type=number]', '1')
        page.check('input[name=resume][value=ask]')
        page.click('section:has(h2:has-text("Playback")) button[type=submit]')
        page.wait_for_selector('[role=status]:has-text("Saved")', timeout=5000)
        # password change validation
        page.fill('section:has(h2:has-text("Password")) input >> nth=0', 'adminpass1')
        page.fill('section:has(h2:has-text("Password")) input >> nth=1', 'newpass123')
        page.fill('section:has(h2:has-text("Password")) input >> nth=2', 'different')
        page.click('button:has-text("Change password")')
        page.wait_for_selector('text=do not match', timeout=5000)
        check('password mismatch is caught', True)

    @step('admin')
    def _():
        page.locator('nav a[href="#/admin"]').click()
        page.wait_for_selector('h1:has-text("Administration")', timeout=5000)
        page.wait_for_selector('section:has(h2:has-text("Libraries")) tbody tr', timeout=5000)
        check('library table lists the library', page.locator('section:has(h2:has-text("Libraries")) tbody tr').count() == 1)
        check('scan status is shown', page.locator('section:has(h2:has-text("Library scan"))').inner_text().count('courses') >= 1, page.locator('section:has(h2:has-text("Library scan"))').inner_text())
        page.click('button:has-text("Scan now")')
        time.sleep(1.5)
        txt = page.locator('section:has(h2:has-text("Library scan"))').inner_text()
        check('scan can be started and finishes', 'Last scan finished' in txt, txt)
        # create a user
        form = page.locator('form:has(button:has-text("Create user"))')
        form.locator('input >> nth=0').fill('bob')
        form.locator('input >> nth=1').fill('Bob')
        form.locator('input[type=password]').fill('bobpass12')
        form.locator('button:has-text("Create user")').click()
        page.wait_for_selector('[role=status]:has-text("Created bob")', timeout=5000)
        check('admin can create a user', page.locator('tbody tr', has_text='bob').count() == 1)
        # self-protection
        row = page.locator('tbody tr', has_text='admin').first
        check('own row cannot be disabled/deleted/demoted', row.locator('button:has-text("Disable")').is_disabled() and row.locator('button:has-text("Delete")').is_disabled() and row.locator('select').is_disabled())

    @step('second user is isolated')
    def _():
        ctx2 = browser.new_context(viewport={'width': 1100, 'height': 900})
        p2 = ctx2.new_page()
        p2.on('console', on_console)
        p2.goto(BASE + '/')
        p2.wait_for_selector('h2:has-text("Sign in")', timeout=8000)
        body = p2.inner_text('body')
        check('login page has no helper text', p2.locator('small, .hint').count() == 0 and 'created by' not in body, body)
        p2.fill('input[name=username]', 'bob')
        p2.fill('input[name=password]', 'wrong-password')
        p2.click('button:has-text("Sign in")')
        p2.wait_for_selector('[role=alert]', timeout=5000)
        check('wrong password shows an error', True)
        p2.fill('input[name=password]', 'bobpass12')
        p2.click('button:has-text("Sign in")')
        p2.wait_for_selector('h1:has-text("Courses")', timeout=8000)
        check('no admin link for a normal user', p2.locator('nav a[href="#/admin"]').count() == 0)
        check('no "continue learning" row for a new user', p2.locator('section[aria-label="Continue learning"]').count() == 0)
        gbl = p2.locator('ul[aria-label="All courses"] li', has_text='Go Basics')
        check("bob's progress starts at zero", '0%' in gbl.inner_text(), gbl.inner_text())
        goto_hash(p2, '#/admin')
        p2.wait_for_selector('text=Only administrators', timeout=5000)
        check('admin page is refused in the UI', True)
        r = api(p2, '/admin/users')
        check('admin API is refused for a normal user', r['status'] == 403, r)
        check("bob cannot see admin's progress", progress(p2, ids['01 - Notes.md']) is None)
        # sign out
        p2.click('button:has-text("Sign out")')
        p2.wait_for_selector('h2:has-text("Sign in")', timeout=5000)
        check('sign out returns to the login page', True)
        r = api(p2, '/me')
        check('session is gone after sign out', r['status'] == 401, r)
        ctx2.close()

    @step('player keyboard shortcuts')
    def _():
        vid = ids['01 - Welcome.webm']
        goto_hash(page, f"#/course/{gb()}/item/{vid}")
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
        st = lambda: page.evaluate('() => { const v = document.querySelector("video"); return {paused: v.paused, t: v.currentTime, vol: v.volume, muted: v.muted, ended: v.ended} }')
        hud = lambda: page.locator('.hud').inner_text() if page.locator('.hud').count() else ''
        # a known starting point: paused at 10 s, volume 50 %, nothing focused
        page.evaluate('() => { const v = document.querySelector("video"); v.pause(); v.muted = false; v.volume = 0.5; v.currentTime = 10; if (document.activeElement) document.activeElement.blur(); }')
        wait_until(lambda: abs(st()['t'] - 10) < 0.5 and st()['paused'] and abs(st()['vol'] - 0.5) < 0.01, 4, desc='parked at 10 s')

        # Space and the arrows work with nothing focused
        page.keyboard.press('Space')
        wait_until(lambda: not st()['paused'], 4, desc='Space plays')
        check('Space starts playback even though nothing is focused', True)
        check('a pill confirms a key press', page.locator('.hud').count() == 1)
        page.keyboard.press('Space')
        wait_until(lambda: st()['paused'], 4, desc='Space pauses')
        check('Space pauses again', True)
        t0 = st()['t']
        page.keyboard.press('ArrowRight')
        wait_until(lambda: abs(st()['t'] - (t0 + 5)) < 0.3, 4, desc='Right skips ahead')
        check('Right arrow skips 5 s ahead', True)
        check('the pill says how far', '5 s' in hud(), hud())
        page.keyboard.press('ArrowLeft')
        wait_until(lambda: abs(st()['t'] - t0) < 0.3, 4, desc='Left skips back')
        check('Left arrow skips 5 s back', True)
        page.keyboard.press('ArrowUp')
        wait_until(lambda: abs(st()['vol'] - 0.55) < 0.011, 3, desc='Up raises the volume')
        check('Up arrow raises the volume by 5 %', True)
        check('the pill shows the volume', '55%' in hud(), hud())
        page.keyboard.press('ArrowDown')
        page.keyboard.press('ArrowDown')
        wait_until(lambda: abs(st()['vol'] - 0.45) < 0.011, 3, desc='Down lowers the volume')
        check('Down arrow lowers the volume by 5 %', True)
        read_saved = lambda: json.loads(page.evaluate('() => localStorage.getItem("lectern-volume") || "{}"'))
        wait_until(lambda: abs(read_saved().get('volume', -1) - 0.45) < 0.011, 3, desc='volume saved')  # "volumechange" arrives a moment after the change
        saved = read_saved()
        check('the volume is remembered on this device', abs(saved['volume'] - 0.45) < 0.011 and saved['muted'] is False, saved)
        check('the page does not scroll when the keys are used', page.evaluate('() => window.scrollY') == 0)

        # a button you clicked with the mouse does not swallow Space ...
        toggle = page.locator('button[aria-controls="course-outline"]')
        expanded = lambda: toggle.get_attribute('aria-expanded')
        toggle.click()
        side_state = expanded()
        was_paused = st()['paused']
        page.keyboard.press('Space')
        wait_until(lambda: st()['paused'] != was_paused, 4, desc='Space after a click')
        check('after clicking a button, Space still plays and pauses', expanded() == side_state, (expanded(), side_state))
        # ... but one you reached with Tab keeps it
        toggle.focus()
        page.keyboard.press('Shift+Tab')
        page.keyboard.press('Tab')
        check('Shift+Tab then Tab brings the focus back to the button', page.evaluate('() => document.activeElement === document.querySelector("button[aria-controls=course-outline]")'))
        before = st()['paused']
        page.keyboard.press('Space')
        wait_until(lambda: expanded() != side_state, 3, desc='Space presses a focused button')
        time.sleep(0.3)
        check('a button reached with Tab is still pressed by Space', st()['paused'] == before)
        page.keyboard.press('Space')  # put the sidebar back
        wait_until(lambda: expanded() == side_state, 3, desc='sidebar restored')

        # with the player itself focused the browser must not also act (no double toggle, no 10 s skip)
        page.evaluate('() => { const v = document.querySelector("video"); v.pause(); v.focus(); }')
        wait_until(lambda: st()['paused'], 2, desc='paused')
        page.keyboard.press('Space')
        wait_until(lambda: not st()['paused'], 3, desc='Space plays with the video focused')
        time.sleep(0.5)
        check('with the player focused, Space toggles exactly once', not st()['paused'])
        page.keyboard.press('Space')
        time.sleep(0.5)
        check('and toggles back exactly once', st()['paused'])
        t1 = st()['t']
        page.keyboard.press('ArrowRight')
        time.sleep(0.6)
        check('with the player focused, an arrow skips 5 s, not 10', abs(st()['t'] - (t1 + 5)) < 0.3, (t1, st()['t']))

        # text fields keep the keys
        page.evaluate("() => { const i = document.createElement('input'); i.id = 'probe'; document.getElementById('main').append(i); i.focus(); }")
        b = st()
        for k in ['Space', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown']:
            page.keyboard.press(k)
        time.sleep(0.4)
        a = st()
        check('typing in a text field does not control the player', a['paused'] == b['paused'] and abs(a['t'] - b['t']) < 0.2 and abs(a['vol'] - b['vol']) < 0.001, (b, a))
        check('the space went into the field', page.input_value('#probe') == ' ')
        page.evaluate("() => document.getElementById('probe').remove()")

        # the volume carries over to the next lesson, and stops at 100 %
        goto_hash(page, f"#/course/{gb()}/item/{ids['02 - Setup.webm']}")
        page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
        check('the next lesson starts at the remembered volume', abs(st()['vol'] - 0.45) < 0.011, st())
        page.evaluate('() => { document.querySelector("video").pause(); if (document.activeElement) document.activeElement.blur(); }')
        for _ in range(14):
            page.keyboard.press('ArrowUp')
        wait_until(lambda: abs(st()['vol'] - 1) < 0.001, 3, desc='volume tops out')
        check('the volume stops at 100 %', True)
        page.keyboard.press('Control+ArrowDown')
        time.sleep(0.3)
        check('Ctrl+arrow is left to the browser', abs(st()['vol'] - 1) < 0.001, st())

        # replaying after the end stops "next lesson in 5 ..."
        for how in ('ArrowLeft', 'Space'):
            goto_hash(page, f"#/course/{gb()}/item/{vid}")
            page.wait_for_function('() => { const v = document.querySelector("video"); return v && v.readyState >= 1 }', timeout=15000)
            page.evaluate('() => { const v = document.querySelector("video"); v.currentTime = v.duration - 1; v.play(); if (document.activeElement) document.activeElement.blur(); }')
            page.wait_for_selector('.countdown', timeout=12000)
            page.keyboard.press(how)
            wait_until(lambda: page.locator('.countdown').count() == 0, 3, desc=f'{how} cancels the countdown')
            check(f'{how} after the end cancels the countdown', True)
        time.sleep(6)
        check('and the lesson does not move on by itself', item_id(page) == vid, item_id(page))
        check('Space after the end plays it again', not st()['paused'] and st()['t'] < 10, st())
        page.evaluate('() => document.querySelector("video").pause()')

    @step('admin: course cover pictures')
    def _():
        from PIL import Image
        import tempfile
        tmp = tempfile.mkdtemp()
        def picture(name, w, h, fmt):
            g = Image.linear_gradient('L').resize((w, h))
            img = Image.merge('RGB', (g, g.transpose(Image.FLIP_LEFT_RIGHT), Image.new('L', (w, h), 170)))
            path = os.path.join(tmp, name)
            img.save(path, fmt)
            return path
        big = picture('big.png', 2400, 1500, 'PNG')
        small = picture('small.png', 320, 200, 'PNG')
        jpg = picture('photo.jpg', 900, 600, 'JPEG')
        txt = os.path.join(tmp, 'notes.txt')
        open(txt, 'w').write('not a picture')
        svg = os.path.join(tmp, 'logo.svg')
        open(svg, 'w').write('<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>')

        pt, gbid = ids['Python Tips'], gb()
        hero_img = page.locator('.hero img')
        change = page.locator('button:has-text("Change cover")')
        remove = page.locator('button[aria-label="Remove the uploaded cover"]')
        course = lambda cid: api(page, f'/courses/{cid}')['body']['course']
        loaded = '() => { const i = document.querySelector(".hero img"); return i && i.complete && i.naturalWidth > 0 }'

        goto_hash(page, f'#/course/{pt}')
        page.wait_for_selector('h1:has-text("Python Tips")', timeout=8000)
        check('an admin gets a "Change cover" button on the course page', change.count() == 1)
        check('there is nothing to remove yet', remove.count() == 0)
        check('a course without a cover shows the drawn artwork', hero_img.count() == 0)

        page.set_input_files('.cover-tools input[type=file]', big)
        page.wait_for_function(loaded, timeout=15000)
        c = course(pt)
        check('the uploaded picture is shown and saved', c['has_cover'] and c['cover_v'] > 0, c)
        dims = page.evaluate('() => { const i = document.querySelector(".hero img"); return [i.naturalWidth, i.naturalHeight, i.getAttribute("src")] }')
        check('a big picture is shrunk in the browser before it is uploaded', dims[0] == 1600 and dims[1] == 1000, dims)
        check('the picture address carries the version', dims[2].endswith(f"?v={c['cover_v']}"), dims[2])
        check('a Remove button appears for an uploaded cover', remove.count() == 1)

        goto_hash(page, '#/')
        page.wait_for_selector('ul[aria-label="All courses"] li', timeout=5000)
        card = page.locator('ul[aria-label="All courses"] li', has_text='Python Tips')
        page.wait_for_function('() => { const l = [...document.querySelectorAll("ul[aria-label=\\"All courses\\"] li")].find(x => x.textContent.includes("Python Tips")); const i = l && l.querySelector("img"); return i && i.complete && i.naturalWidth > 0 }', timeout=8000)
        check('the gallery shows the new cover', card.locator('img').count() == 1)

        goto_hash(page, f'#/course/{pt}')
        page.wait_for_function(loaded, timeout=8000)
        v_before = course(pt)['cover_v']
        time.sleep(0.01)
        page.set_input_files('.cover-tools input[type=file]', small)
        wait_until(lambda: course(pt)['cover_v'] != v_before, 8, desc='replacement saved')
        page.wait_for_function('() => { const i = document.querySelector(".hero img"); return i && i.complete && i.naturalWidth === 320 }', timeout=8000)
        check('a small picture is sent as it is, and replaces the old one at once', True)
        page.set_input_files('.cover-tools input[type=file]', jpg)
        wait_until(lambda: course(pt)['cover_v'] != v_before and page.evaluate('() => { const i = document.querySelector(".hero img"); return i && i.complete && i.naturalWidth === 900 }'), 8, desc='jpeg saved')
        check('a JPEG works too', True)

        v_now = course(pt)['cover_v']
        for bad in (txt, svg):
            page.set_input_files('.cover-tools input[type=file]', bad)
            page.wait_for_selector('.hero [role=alert]:has-text("JPEG, PNG, WebP or AVIF")', timeout=4000)
            check(f'{os.path.basename(bad)} is refused with a clear message', course(pt)['cover_v'] == v_now)

        remove.click()
        wait_until(lambda: not course(pt)['has_cover'], 6, desc='cover removed')
        wait_until(lambda: hero_img.count() == 0, 4, desc='artwork returns')
        c = course(pt)
        check('Remove brings back the drawn artwork', c['cover_v'] == 0 and remove.count() == 0, c)

        # a course whose folder has its own cover image: the upload wins, removing it restores the folder image
        goto_hash(page, f'#/course/{gbid}')
        page.wait_for_selector('h1:has-text("Go Basics")', timeout=8000)
        page.wait_for_function(loaded, timeout=8000)
        folder_src = page.evaluate('() => document.querySelector(".hero img").getAttribute("src")')
        check('a folder image counts as a cover but is not "uploaded"', course(gbid)['has_cover'] and course(gbid)['cover_v'] == 0 and remove.count() == 0 and '?v=' not in folder_src, folder_src)
        page.set_input_files('.cover-tools input[type=file]', small)
        wait_until(lambda: course(gbid)['cover_v'] > 0, 8, desc='upload over the folder image')
        page.wait_for_function('() => { const i = document.querySelector(".hero img"); return i && i.complete && i.naturalWidth === 320 }', timeout=8000)
        check('an uploaded cover outranks the folder image', True)
        remove.click()
        wait_until(lambda: course(gbid)['cover_v'] == 0, 6, desc='back to the folder image')
        page.wait_for_function('() => { const i = document.querySelector(".hero img"); return i && i.complete && i.naturalWidth > 0 && i.naturalWidth !== 320 }', timeout=8000)
        check('removing it brings the folder image back', course(gbid)['has_cover'])

        # everybody sees it; only admins can change it
        goto_hash(page, f'#/course/{pt}')
        page.wait_for_selector('h1:has-text("Python Tips")', timeout=8000)
        page.set_input_files('.cover-tools input[type=file]', jpg)
        wait_until(lambda: course(pt)['cover_v'] > 0, 8, desc='cover for bob to see')
        ctx_b = browser.new_context(viewport={'width': 1100, 'height': 900})
        pb = ctx_b.new_page()
        pb.on('console', on_console)
        pb.goto(BASE + '/')
        pb.wait_for_selector('h2:has-text("Sign in")', timeout=8000)
        pb.fill('input[name=username]', 'bob')
        pb.fill('input[name=password]', 'bobpass12')
        pb.click('button:has-text("Sign in")')
        pb.wait_for_selector('h1:has-text("Courses")', timeout=8000)
        pb.wait_for_function('() => { const l = [...document.querySelectorAll("ul[aria-label=\\"All courses\\"] li")].find(x => x.textContent.includes("Python Tips")); const i = l && l.querySelector("img"); return i && i.complete && i.naturalWidth === 900 }', timeout=8000)
        check('other users see the uploaded cover in the gallery', True)
        goto_hash(pb, f'#/course/{pt}')
        pb.wait_for_selector('h1:has-text("Python Tips")', timeout=8000)
        check('a normal user has no cover controls', pb.locator('button:has-text("Change cover")').count() == 0 and pb.locator('input[type=file]').count() == 0)
        r = api(pb, f'/admin/courses/{pt}/cover', 'DELETE')
        check('and the server refuses them anyway', r['status'] == 403, r)
        check("the cover survived the attempt", course(pt)['cover_v'] > 0)
        ctx_b.close()

        remove.click()
        wait_until(lambda: course(pt)['cover_v'] == 0, 6, desc='cleanup')

    @step('a folder of only videos is a plain list')
    def _():
        fv = ids['Flat Videos']
        tree = api(page, f'/courses/{fv}')['body']
        check('the server marks the only section as loose', len(tree['sections']) == 1 and tree['sections'][0]['loose'] is True, tree['sections'])
        goto_hash(page, f'#/course/{fv}')
        page.wait_for_selector('h1:has-text("Flat Videos")', timeout=5000)
        rows = page.locator('ol[aria-label="Lessons"] > li')
        wait_until(lambda: rows.count() == 2, 5, desc='both videos are listed')
        check('no section heading above loose videos', page.locator('main h3').count() == 0, page.locator('main h3').all_inner_texts())
        check('the made-up "Course files" name is not shown', 'Course files' not in page.inner_text('main'))
        check('the videos are listed in order', 'First' in rows.nth(0).inner_text() and 'Second' in rows.nth(1).inner_text(), rows.all_inner_texts())
        check('progress counts the videos', '0/2 items' in page.inner_text('main'), page.inner_text('main')[:300])
        page.locator('a', has_text='Start course').click()
        wait_until(lambda: item_id(page) == ids['01 - First.webm'], 8, desc='Start opens the first video')
        toggle = page.locator('button[aria-controls="course-outline"]')
        side = page.locator('#course-outline')
        toggle.wait_for(timeout=8000)
        if toggle.get_attribute('aria-expanded') != 'true':
            toggle.click()
        side.wait_for(state='visible', timeout=4000)
        wait_until(lambda: side.locator('ol[aria-label="Lessons"] > li').count() == 2, 6, desc='the sidebar lists the videos')
        check('the sidebar is a plain list too', side.locator('button[aria-controls^="sec-"]').count() == 0 and side.locator('h3').count() == 0)
        check('the lesson header has no section chip', 'Course files' not in page.locator('.lesson-head').inner_text(), page.locator('.lesson-head').inner_text())
        page.locator('nav[aria-label="Lesson navigation"] a[rel=next]').click()
        wait_until(lambda: item_id(page) == ids['02 - Second.webm'], 8, desc='Next opens the second video')
        check('Next goes on to the second video', True)
        api(page, f"/items/{ids['01 - First.webm']}/progress", 'POST', {'event': 'done', 'done': True, 'client_ts': int(time.time() * 1000)})
        goto_hash(page, f'#/course/{fv}')
        page.wait_for_selector('text=50% complete', timeout=6000)
        check('finishing one of two videos makes the course half done', '1/2 items' in page.inner_text('main'), page.inner_text('main')[:300])

    @step('a video lying in the library root is a course, with chapters')
    def _():
        ll, lid = ids['Long Lecture'], ids['Long Lecture.webm']
        item = lambda: api(page, f'/items/{lid}')['body']['item']
        vt = lambda: page.evaluate('() => document.querySelector("video").currentTime')
        goto_hash(page, '#/')
        page.wait_for_selector('ul[aria-label="All courses"] li', timeout=5000)
        card = page.locator('ul[aria-label="All courses"] li', has_text='Long Lecture')
        check('the lone video is a course in the gallery', card.count() == 1)
        page.wait_for_function('() => { const l = [...document.querySelectorAll("ul[aria-label=\\"All courses\\"] li")].find(x => x.textContent.includes("Long Lecture")); const i = l && l.querySelector("img"); return i && i.complete && i.naturalWidth > 0 }', timeout=8000)
        check('the picture next to the video is its cover', True)
        page.locator('ul[aria-label="All courses"] a', has_text='Long Lecture').click()
        page.wait_for_selector('h1:has-text("Long Lecture")', timeout=5000)
        chl = page.locator('ol[aria-label="Chapters of Long Lecture"] > li')
        wait_until(lambda: chl.count() == 3, 5, desc='the chapters are listed')
        texts = chl.all_inner_texts()
        check('chapters are listed with their start times and names',
              '0:00' in texts[0] and 'Opening' in texts[0] and '0:30' in texts[1] and 'Middle part' in texts[1] and '1:00' in texts[2] and 'Closing' in texts[2], texts)
        check('each chapter counts as an item', '0/3 items' in page.inner_text('main'), page.inner_text('main')[:300])
        check('a lone video has no section heading', page.locator('main h3').count() == 0)
        # ticking chapters on the course page
        chl.nth(0).locator('button.tick').click()
        page.wait_for_selector('text=1/3 items', timeout=6000)
        check('a chapter can be ticked from the course page', item()['chapters'][0]['done'] is True, item())
        chl.nth(0).locator('button.tick').click()
        page.wait_for_selector('text=0/3 items', timeout=6000)
        check('and un-ticked again', item()['chapters'][0]['done'] is False)

        # a chapter opens the video at its start
        chl.nth(1).locator('a').click()
        wait_until(lambda: item_id(page) == lid, 8, desc='the chapter opens the video')
        page.wait_for_selector('video', timeout=8000)
        wait_until(lambda: abs(vt() - 30) < 1.5, 8, desc='the video starts at the chapter')
        check('the video opens at the chapter that was clicked', True)
        check('the address names the time', 't=30' in page.evaluate('location.hash'), page.evaluate('location.hash'))
        check('no resume prompt when a chapter was chosen', page.locator('section[aria-label="Resume"]').count() == 0)
        head = page.locator('.lesson-head')
        wait_until(lambda: 'Chapter 2 of 3' in head.inner_text(), 6, desc='the header names the chapter')
        check('the header says which chapter is playing', 'Middle part' in head.inner_text(), head.inner_text())
        check('the header has no section chip for a lone video', 'Course files' not in head.inner_text())

        # the sidebar lists the chapters under the video
        toggle = page.locator('button[aria-controls="course-outline"]')
        side = page.locator('#course-outline')
        toggle.wait_for(timeout=8000)
        if toggle.get_attribute('aria-expanded') != 'true':
            toggle.click()
        side.wait_for(state='visible', timeout=4000)
        side_ch = side.locator('ol[aria-label="Chapters of Long Lecture"] > li')
        wait_until(lambda: side_ch.count() == 3, 6, desc='the sidebar lists the chapters')
        check('the sidebar lists the chapters under the video', True)
        check('the playing chapter is marked there', side.locator('li[data-now="true"]').count() == 1 and 'Middle part' in side.locator('li[data-now="true"]').inner_text(), side.locator('li[data-now="true"]').all_inner_texts())
        check('chapters are not listed twice while the sidebar shows them', page.locator('section.chapters').count() == 0)
        side_ch.nth(0).locator('a').click()
        wait_until(lambda: vt() < 1.5, 6, desc='the sidebar jumps to the first chapter')
        check('a chapter in the sidebar jumps in place and plays', 't=30' in page.evaluate('location.hash') and page.evaluate('() => !document.querySelector("video").paused'), page.evaluate('location.hash'))
        wait_until(lambda: 'Chapter 1 of 3' in head.inner_text(), 6, desc='the header follows')
        check('the header follows the jump', 'Opening' in head.inner_text(), head.inner_text())

        # with the sidebar closed, the chapters sit under the player
        toggle.click()
        page.wait_for_selector('section.chapters', timeout=4000)
        panel = page.locator('section.chapters')
        pch = panel.locator('ol > li')
        check('with the sidebar closed the chapters sit under the player', pch.count() == 3)
        pch.nth(2).locator('a').click()
        wait_until(lambda: abs(vt() - 60) < 1.5, 6, desc='the panel jumps to the last chapter')
        wait_until(lambda: 'Chapter 3 of 3' in head.inner_text(), 6, desc='the header follows again')
        check('a chapter under the player jumps there', True)
        check('and the chapter that was left behind is not counted as watched', not any(c['done'] for c in item()['chapters']), item()['chapters'])

        # playing through the end of a chapter finishes it
        page.evaluate('() => { const v = document.querySelector("video"); v.currentTime = 27; v.play(); }')
        wait_until(lambda: item()['chapters'][0]['done'], 15, desc='the first chapter finishes when played through')
        check('playing through the end of a chapter finishes it', True)
        its = item()
        check('chapters that were only jumped over stay unfinished', [c['done'] for c in its['chapters']] == [True, False, False], its['chapters'])
        check('the video is started, not done', its['progress']['state'] == 'started', its['progress'])
        wait_until(lambda: pch.nth(0).locator('button[aria-pressed="true"]').count() == 1, 4, desc='the tick shows')
        cs = api(page, f'/courses/{ll}')['body']['course']
        check('the course counts finished chapters', cs['done_count'] == 1 and cs['item_count'] == 3, cs)

        # by hand
        page.evaluate('() => document.querySelector("video").pause()')
        pch.nth(1).locator('button.tick').click()
        wait_until(lambda: item()['chapters'][1]['done'], 8, desc='a ticked chapter is saved')
        check('a chapter can be ticked by hand', True)
        pch.nth(1).locator('button.tick').click()
        wait_until(lambda: not item()['chapters'][1]['done'], 8, desc='an un-ticked chapter is saved')
        check('and un-ticked', True)
        pch.nth(1).locator('button.tick').click()
        wait_until(lambda: item()['chapters'][1]['done'], 8, desc='ticked again')

        # the last chapter finishes at the end of the video, and with it the video
        page.evaluate('() => { const v = document.querySelector("video"); v.currentTime = v.duration - 2.5; v.play(); }')
        wait_until(lambda: item()['progress']['state'] == 'done', 20, desc='the last chapter finishes the video')
        its = item()
        check('playing to the end finished the last chapter, and with it the video', all(c['done'] for c in its['chapters']) and its['progress']['state'] == 'done', its)
        wait_until(lambda: 'Completed' in head.inner_text(), 6, desc='the page says completed')
        check('the page says it is completed', True)
        check('the course is complete', api(page, f'/courses/{ll}')['body']['completed'] is True)

        # taking it back clears every chapter
        page.locator('button', has_text='Mark not done').click()
        wait_until(lambda: not any(c['done'] for c in item()['chapters']) and item()['progress']['state'] == 'started', 8, desc='Mark not done clears the chapters')
        check('"Mark not done" clears every chapter', True)
        check('and offers "Mark all done"', page.locator('button', has_text='Mark all done').count() == 1)
        check('the ticks are cleared on screen', panel.locator('button[aria-pressed="true"]').count() == 0)
        page.locator('button', has_text='Mark all done').click()
        wait_until(lambda: item()['progress']['state'] == 'done' and all(c['done'] for c in item()['chapters']), 8, desc='Mark all done')
        check('"Mark all done" finishes every chapter', True)
        page.locator('button', has_text='Mark not done').click()
        wait_until(lambda: not any(c['done'] for c in item()['chapters']), 8, desc='cleared again')

        # what is ticked is still ticked after a reload
        pch.nth(2).locator('button.tick').click()
        wait_until(lambda: item()['chapters'][2]['done'], 8, desc='the last chapter ticked')
        page.reload()
        page.wait_for_selector('section.chapters', timeout=8000)
        after = page.locator('section.chapters ol > li')
        wait_until(lambda: after.count() == 3, 6, desc='chapters back after reload')
        check('ticked chapters are still ticked after a reload',
              after.nth(2).locator('button[aria-pressed="true"]').count() == 1 and after.nth(0).locator('button[aria-pressed="true"]').count() == 0)
        toggle = page.locator('button[aria-controls="course-outline"]')
        toggle.click()  # leave the sidebar open, as the other steps expect
        page.locator('#course-outline').wait_for(state='visible', timeout=4000)

        # a phone: chapters under the player, and in the drawer
        ctx4 = browser.new_context(viewport={'width': 390, 'height': 844}, is_mobile=True, has_touch=True)
        p4 = ctx4.new_page()
        p4.on('console', on_console)
        p4.on('pageerror', lambda e: console_errors.append(f'pageerror: {e}'))
        p4.goto(BASE + '/')
        p4.wait_for_selector('h2:has-text("Sign in")', timeout=8000)
        p4.fill('input[name=username]', 'admin')
        p4.fill('input[name=password]', 'adminpass1')
        p4.click('button:has-text("Sign in")')
        p4.wait_for_selector('h1:has-text("Courses")', timeout=8000)
        goto_hash(p4, f'#/course/{ll}/item/{lid}')
        p4.wait_for_selector('section.chapters', timeout=8000)
        check('on a phone the chapters are listed under the player', p4.locator('section.chapters ol > li').count() == 3)
        check('the lesson does not scroll sideways on a phone', p4.evaluate('() => document.documentElement.scrollWidth <= window.innerWidth + 1'))
        side4 = p4.locator('#course-outline')
        p4.locator('button[aria-controls="course-outline"]').click()
        side4.wait_for(state='visible', timeout=4000)
        drawer_ch = side4.locator('ol[aria-label="Chapters of Long Lecture"] > li')
        wait_until(lambda: drawer_ch.count() == 3, 6, desc='the drawer lists the chapters')
        check('the drawer lists the chapters under the video, not a second list behind it', p4.locator('section.chapters').count() == 0)
        drawer_ch.nth(1).locator('a').click()
        wait_until(lambda: not side4.is_visible(), 4, desc='choosing a chapter closes the drawer')
        wait_until(lambda: abs(p4.evaluate('() => document.querySelector("video").currentTime') - 30) < 1.5, 6, desc='and jumps there')
        check('a chapter chosen in the drawer closes it and jumps there', True)
        ctx4.close()

    @step('course completion state')
    def _():
        # finish everything in Go Basics as admin via the API, then look at the course page
        tree = api(page, f'/courses/{gb()}')['body']
        for sec in tree['sections']:
            for it in sec['items']:
                api(page, f"/items/{it['id']}/progress", 'POST', {'event': 'done', 'done': True, 'client_ts': int(time.time() * 1000)})
        goto_hash(page, f'#/course/{gb()}')
        page.wait_for_selector('text=You have completed this course', timeout=8000)
        check('completed course says so', True)
        goto_hash(page, '#/')
        page.wait_for_selector('ul[aria-label="All courses"] li', timeout=5000)
        check('completed course is dropped from "Continue learning"', page.locator('section[aria-label="Continue learning"]', has_text='Go Basics').count() == 0)
        check('home shows 100%', '100%' in page.locator('ul[aria-label="All courses"] li', has_text='Go Basics').inner_text())

    @step('not found and deep links')
    def _():
        goto_hash(page, '#/nope')
        page.wait_for_selector('h1:has-text("Page not found")', timeout=5000)
        check('unknown route shows not-found', True)
        goto_hash(page, '#/course/99999')
        page.wait_for_selector('[role=alert]', timeout=5000)
        check('missing course shows an error', True)
        goto_hash(page, f"#/course/{ids['Python Tips']}/item/{ids['01 - Notes.md']}")
        page.wait_for_selector('[role=alert]:has-text("not part of that course")', timeout=5000)
        check('item under the wrong course is refused', True)
        # a hard reload of a deep link keeps the session
        goto_hash(page, f"#/course/{gb()}")
        page.reload()
        page.wait_for_selector('h1:has-text("Go Basics")', timeout=8000)
        check('reload on a deep link stays signed in', True)

    browser.close()

ok = sum(1 for r in results if r[0])
bad = [r for r in results if not r[0]]
print(f'\n{ok}/{len(results)} checks passed')
unexpected = [e for e in console_errors if not ('401' in e and '/api/v1/' in e) and not ('404' in e and '/api/v1/courses/99999' in e) and not ('403' in e and '/admin/users' in e) and not ('403' in e and '/admin/courses/' in e) and 'status of 404' not in e]
print('console errors (unexpected):', json.dumps(unexpected, indent=1) if unexpected else 'none')
print('console errors (all):', len(console_errors))
for e in console_errors:
    print('   ', e[:200])
sys.exit(1 if bad else 0)
