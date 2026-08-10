# Music

Two files, both optional, both declared in the `TRACKS` table at the top of the
sound section in `web/app.js`:

- **`intro.mp3`** — loops over the title and lobby screens while people are
  arriving, and fades out when the cards are dealt. Around 12 seconds, and it
  wants to loop cleanly since `loop` is true.
- **`theme_song1.mp3`** — plays once over the game-over screen. Length is up to
  you; it is not looped, so it can be the full track.

Nothing plays during a turn. With one phone per player a continuous bed is five
copies of the same track drifting out of sync in one room, so music is kept to
the moments when every device is on the same screen at once. If you want to
change that, `syncSound()` is the only place that decides.

Either file may be absent: the request 404s and the game stays silent, so the
sound toggle is the only thing you will notice.

Keep them mp3 — `static/static.go` embeds `src/audio/*.mp3` and nothing else, so
anything with another extension (a `source.m4a` you cut a clip from) is ignored
by the build and by git. At least one mp3 must exist or the build fails, which is
deliberate: removing the last one should be a decision, not a surprise.

## Producing the clip

Cutting the first 12 seconds of a source track, once you have a file you hold
the rights to use:

```sh
ffmpeg -i source.m4a -ss 0 -t 12 -ac 1 -b:a 128k intro.mp3
```

If the source is a YouTube video, `yt-dlp` will fetch the audio for you:

```sh
yt-dlp -x --audio-format m4a -o source.m4a '<url>'
```

Note that most board-game music on YouTube is someone else's copyrighted
recording, and re-distributing it inside this repository is a separate question
from playing it on your own machine. For anything you intend to share, use a
track you own or one under a licence that permits it.

## Volume and looping

Per track, in `web/app.js`:

```js
const TRACKS = {
  intro: { src: "/audio/intro.mp3", volume: 0.55, loop: true },
  theme: { src: "/audio/theme_song1.mp3", volume: 0.6, loop: false },
};
```

The mute state is remembered per browser under the `ek:muted` localStorage key.
