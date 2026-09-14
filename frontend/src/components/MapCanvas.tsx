import { useEffect, useRef } from 'react'
import Map from 'ol/Map.js'
import View from 'ol/View.js'
import TileLayer from 'ol/layer/Tile.js'
import VectorLayer from 'ol/layer/Vector.js'
import VectorSource from 'ol/source/Vector.js'
import OSM from 'ol/source/OSM.js'
import Feature from 'ol/Feature.js'
import Point from 'ol/geom/Point.js'
import { fromLonLat } from 'ol/proj.js'
import { Icon, Style } from 'ol/style.js'
import Overlay from 'ol/Overlay.js'

export type MapMarker = {
  id: string | number
  name: string
  lat: number
  lon: number
  detail?: string
  color?: string
}

type MapCanvasProps = {
  markers: MapMarker[]
  height?: number
}

const DEFAULT_COLOR = '#2563eb'

function iconHref(color: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(`
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="36" viewBox="0 0 32 36">
      <path fill="${color}" stroke="#ffffff" stroke-width="2" d="M16 0C7.2 0 0 7.2 0 16c0 12 16 20 16 20s16-8 16-20C32 7.2 24.8 0 16 0z"/>
      <circle cx="16" cy="16" r="6" fill="#ffffff"/>
    </svg>
  `)}`
}

export function MapCanvas({ markers, height = 320 }: MapCanvasProps) {
  const ref = useRef<HTMLDivElement>(null)
  const busy = useRef(false)

  useEffect(() => {
    if (!ref.current || busy.current) return
    busy.current = true

    const markerFeatures = markers.map((m, i) => {
      const f = new Feature({ geometry: new Point(fromLonLat([m.lon, m.lat])) })
      f.set('__idx', i)
      f.setStyle(
        new Style({ image: new Icon({ src: iconHref(m.color ?? DEFAULT_COLOR), scale: 0.9 }) }),
      )
      return f
    })

    const vector = new VectorLayer({ source: new VectorSource({ features: markerFeatures }) })

    const map = new Map({
      target: ref.current,
      layers: [new TileLayer({ source: new OSM() }), vector],
      view: new View({ center: fromLonLat([106.8456, -6.2088]), zoom: 11 }),
      controls: [],
    })

    const extent = vector.getSource()?.getExtent()
    if (extent) {
      map.getView().fit(extent, {
        padding: [50, 50, 50, 50],
        maxZoom: 15,
        duration: 0,
      })
    }

    const popup = document.createElement('div')
    popup.className = 'map-popup'
    map.addOverlay(new Overlay({ element: popup, positioning: 'bottom-center', offset: [0, -12] }))

    const show = (m: MapMarker, pos: [number, number]) => {
      popup.innerHTML = `<strong>${esc(m.name)}</strong>${m.detail ? `<span>${esc(m.detail)}</span>` : ''}`
      map.getOverlays().getArray().find((o) => o.getElement() === popup)?.setPosition(pos)
      popup.classList.add('visible')
    }

    map.on('singleclick', (e) => {
      const f = map.forEachFeatureAtPixel(
        e.pixel,
        (candidate) => candidate,
        { hitTolerance: 8 },
      )
      if (f) {
        const pos: [number, number] = [e.coordinate[0], e.coordinate[1]]
        show(markers[f.get('__idx') as number], pos)
      } else {
        popup.classList.remove('visible')
      }
    })

    map.on('pointermove', (e) => {
      const hit = map.hasFeatureAtPixel(e.pixel, { hitTolerance: 8 })
      map.getTargetElement().style.cursor = hit ? 'pointer' : ''
    })

    const onResize = () => map.updateSize()
    window.addEventListener('resize', onResize)

    return () => {
      window.removeEventListener('resize', onResize)
      map.setTarget(undefined)
      busy.current = false
    }
  }, [markers])

  return <div ref={ref} className="map-canvas" style={{ height }} />
}

function esc(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c] as string))
}

export default MapCanvas