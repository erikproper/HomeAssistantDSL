# Raspberry Pi photo frame with Music Assistant artwork

## Goal

A Raspberry Pi with its own display shows a full-screen photo slideshow. When music plays through a selected Music Assistant player, the display switches to the current album artwork. When playback stops, it returns to the slideshow.

No browser tabs, address bar, window borders, Home Assistant sidebar or dashboard header should be visible.

## Proposed setup

| Component | Role |
| --- | --- |
| Raspberry Pi with attached display | Runs the display interface |
| Raspberry Pi OS Desktop | Operating system for the Pi |
| Chromium in kiosk mode | Opens the dashboard full-screen and starts automatically after login |
| Home Assistant | Hosts the dashboard and exposes the selected player's playback state |
| Music Assistant | Supplies playback information and available album artwork through its Home Assistant integration |
| Photo storage | Home Assistant machine or a NAS, with Nextcloud as another possible source |

Home Assistant and Music Assistant can run on a separate machine. The Pi only needs to load and render the dashboard over the network.

## Display behaviour

- While the selected player is idle or stopped, show the photo slideshow.
- While it is playing, show the current album artwork full-screen.
- Optionally include the artist, album and track title.
- Update the artwork when the track changes.
- Return to the slideshow after playback stops.
- For paused playback, choose whether to keep the artwork visible or return to photos after a timeout.
- If artwork is unavailable, use a configured fallback, such as the photo slideshow.

This behaviour requires a slideshow component and dashboard visibility conditions or equivalent custom-card logic. The exact components and configuration still need to be selected and tested together.

## Full-screen presentation

Configure Chromium to launch in kiosk mode and open a dedicated Home Assistant dashboard automatically. Kiosk mode hides the browser interface.

Configure the dashboard separately to hide Home Assistant's header and sidebar. This may require a community kiosk extension. Use a dedicated Home Assistant user for the display.

Configure the Pi's screen blanking and power settings to match the desired display schedule.

## Photo storage

### Preferred option — Home Assistant machine or NAS

Keep the photo collection on the Home Assistant machine or a NAS accessible to it. The slideshow loads the images over the network to the Pi.

The chosen slideshow component determines how the photo folder is indexed and served. A NAS folder may need to be mounted or exposed through a supported media source.

### Alternative — photos on the Pi

Photos can be stored on the Pi. For a Home Assistant dashboard to load them, a small web server on the Pi would need to serve them at URLs accessible to the browser. HTTPS and browser access requirements must be checked against the dashboard's setup.

### Alternative — Nextcloud

If the photos are already stored in Nextcloud, select a slideshow component with suitable Nextcloud support or synchronize a chosen photo folder to the Home Assistant machine or NAS. Direct access depends on the component's authentication and source support.

## Music integration

Add the Music Assistant integration to Home Assistant and select the player or group whose playback should control the display. Use that entity's state and available artwork in the dashboard.

Artwork availability depends on the music source and metadata. Test the actual player and music services before finalizing the layout.

## Decisions still needed

- Raspberry Pi model and display resolution and orientation.
- Photo source and slideshow component.
- Music Assistant player or group to follow.
- Whether to show track information alongside the artwork.
- Behaviour when playback is paused or artwork is missing.
- Photo change interval, transitions and image cropping.
- Display on/off schedule.

## Implementation checks

1. Verify photos display correctly from the selected storage location.
2. Verify the selected Music Assistant entity exposes playback state and accessible artwork.
3. Configure and test the slideshow and artwork views, including pause and missing-artwork behaviour.
4. Hide the Home Assistant interface controls and enable Chromium kiosk mode.
5. Verify the dashboard opens automatically after a Pi reboot.
6. Test recovery after a temporary network interruption or Home Assistant restart.

## References

- [Music Assistant integration for Home Assistant](https://www.home-assistant.io/integrations/music_assistant/)
- [Music Assistant artwork documentation](https://www.music-assistant.io/metadata/artwork/)
- [Home Assistant media sources](https://www.home-assistant.io/integrations/media_source/)
- [Community Photo Frame dashboard card](https://github.com/ha-workshop/lovelace-photoframe)

This document records the proposed design. Installation and configuration have not yet been performed.

## Alternative: a native display client (Erik, 2026-11-25)

Instead of Chromium on a Home Assistant dashboard: extend feh (or drive it from a program of our
own), with an MQTT connection, and let it take part in Music Assistant as a display-only client.

From a chat with ChatGPT; not verified yet, check against the Sendspin specification and Music
Assistant's implementation first:

- Sendspin (Music Assistant's own streaming protocol) lets a client advertise roles independently,
  without the audio player role: `artwork` (album or artist images), `metadata` (title, artist,
  album, playback progress), `visualizer` (timestamped audio analysis: spectrum, loudness, beats).
- So a Raspberry Pi could show the artwork, and visuals in time with the music, while another
  device plays the audio.
- Music Assistant's development version accounts for display and visualiser clients. A ready-made
  Pi application has not been found; it may have to be built.
- Lyrics need something else: the documented roles have no lyrics feed.
- The limitation of Google Cast concerns its receiver environment, and doesn't apply here.

Compared with Chromium (considerations, not tested):

- The MQTT connection would carry the three controls (on/off, photo_only/mixed, collection)
  straight over the house's bus, so the frame would need no dashboard, and mqtt_commandline no
  scripts.
- It needs far less memory than Chromium: the current Raspberry Pi 3's may do.
- It is code of our own to write and maintain. Photos and artwork may only need feh driven from
  outside; a visualiser needs a renderer, which feh isn't.
- To check: whether a display-only client can follow a player that isn't a Sendspin player itself,
  such as a Sonos pair. If it can't, the artwork could come from Home Assistant's media player.
