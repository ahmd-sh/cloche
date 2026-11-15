package sh.ahmd.cloche.link

import sh.ahmd.cloche.store.Computer
import sh.ahmd.cloche.store.labels

object Links {
    fun of(computers: List<Computer>, links: Map<String, Link>): List<Pair<Computer, Link>> =
        computers.map { it to (links[it.id] ?: Link.Connecting) }

    fun overall(computers: List<Computer>, links: Map<String, Link>): Link {
        if (computers.isEmpty()) return Link.Unpaired
        val each = of(computers, links).map { it.second }
        if (Link.NoTailnet in each) return Link.NoTailnet
        each.filterIsInstance<Link.Live>().minByOrNull { it.since }?.let { return it }
        each.filterIsInstance<Link.Down>().minByOrNull { it.retry }?.let { return it }
        return Link.Connecting
    }

    fun title(computers: List<Computer>, links: Map<String, Link>): String {
        val each = of(computers, links)
        if (each.isEmpty()) return "Not paired"
        if (each.any { it.second == Link.NoTailnet }) return "Tailscale is off"
        val names = labels(computers)
        if (each.size == 1) {
            val (c, link) = each.single()
            return when (link) {
                is Link.Live -> "Linked to ${names[c.id]}"
                is Link.Down -> "Can't reach ${names[c.id]}"
                else -> "Connecting to ${names[c.id]}"
            }
        }
        val live = each.count { it.second is Link.Live }
        val down = each.filter { it.second is Link.Down }.map { names[it.first.id] }
        return when {
            live == each.size -> "Linked to ${each.size} computers"
            live > 0 && down.size == 1 -> "Linked to $live of ${each.size} · can't reach ${down.single()}"
            live > 0 -> "Linked to $live of ${each.size} computers"
            down.size == each.size -> "Can't reach your computers"
            else -> "Connecting to ${each.size} computers"
        }
    }

    fun up(computers: List<Computer>, links: Map<String, Link>): Boolean? = when (overall(computers, links)) {
        is Link.Live -> true
        is Link.Down, Link.NoTailnet -> false
        Link.Connecting, Link.Unpaired -> null
    }

    const val HOUR = 3_600_000L
    const val SLOW = 600_000L
    private val BACKOFF = longArrayOf(1, 2, 5, 10, 20, 30, 60)

    fun backoff(failures: Int, downFor: Long): Long =
        if (downFor >= HOUR) SLOW else BACKOFF[(failures - 1).coerceIn(0, BACKOFF.lastIndex)] * 1000L
}
