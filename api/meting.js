import Meting from '@meting/core';

export default async function handler(req, res) {
  const { server = 'netease', type = 'search', id, format = '1' } = req.query;

  if (req.method === 'OPTIONS') {
    res.status(200).end();
    return;
  }

  try {
    const meting = new Meting(server);

    if (format === '1' || format === 'true') {
      meting.format(true);
    }

    let result;

    switch (type) {
      case 'search': {
        const { page = 1, limit = 30 } = req.query;
        result = await meting.search(id, {
          page: parseInt(page),
          limit: parseInt(limit),
        });
        break;
      }
      case 'song':
        result = await meting.song(id);
        break;
      case 'lyric':
        result = await meting.lyric(id);
        break;
      case 'url': {
        const { br = 320 } = req.query;
        result = await meting.url(id, parseInt(br));
        break;
      }
      case 'pic': {
        const { size = 300 } = req.query;
        result = await meting.pic(id, parseInt(size));
        break;
      }
      case 'artist': {
        const { limit = 50 } = req.query;
        result = await meting.artist(id, parseInt(limit));
        break;
      }
      case 'album':
        result = await meting.album(id);
        break;
      case 'playlist':
        result = await meting.playlist(id);
        break;
      default:
        res.status(400).json({ error: 'Unknown type: ' + type });
        return;
    }

    if (typeof result === 'string') {
      try {
        result = JSON.parse(result);
      } catch (e) {
        // keep as string
      }
    }

    res.setHeader('Cache-Control', 's-maxage=3600, stale-while-revalidate');
    res.status(200).json(result);
  } catch (error) {
    res.status(500).json({
      error: error.message || 'Internal Server Error',
    });
  }
}
