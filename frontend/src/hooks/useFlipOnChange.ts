import { useEffect, useRef, useState } from 'react'

const FLIP_DURATION_MS = 460

export function useFlipOnChange(value: string | number): boolean {
  const previous = useRef(value)
  const [flipping, setFlipping] = useState(false)

  useEffect(() => {
    if (previous.current === value) {
      return
    }
    previous.current = value
    setFlipping(true)
    const timeout = setTimeout(() => setFlipping(false), FLIP_DURATION_MS)
    return () => clearTimeout(timeout)
  }, [value])

  return flipping
}
