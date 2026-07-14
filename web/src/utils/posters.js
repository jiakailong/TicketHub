import livePoster from '../assets/posters/live.jpg'
import festivalPoster from '../assets/posters/festival.jpg'
import dramaPoster from '../assets/posters/drama.jpg'

const posters = [livePoster, festivalPoster, dramaPoster]
const fixed = { '10001': livePoster, '10002': festivalPoster, '10003': dramaPoster }

export function posterFor(id = '') {
  if (fixed[String(id)]) return fixed[String(id)]
  const sum = [...String(id)].reduce((total, char) => total + char.charCodeAt(0), 0)
  return posters[sum % posters.length]
}
