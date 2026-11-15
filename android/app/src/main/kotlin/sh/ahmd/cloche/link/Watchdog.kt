package sh.ahmd.cloche.link

import android.app.job.JobInfo
import android.app.job.JobParameters
import android.app.job.JobScheduler
import android.app.job.JobService
import android.content.ComponentName
import android.content.Context
import android.util.Log
import sh.ahmd.cloche.cloche

class Watchdog : JobService() {
    override fun onStartJob(params: JobParameters): Boolean {
        val app = cloche
        when {
            app.store.value.computers.isEmpty() -> cancel(this)
            LinkService.running -> app.kick()
            else -> runCatching { LinkService.start(this) }.onFailure { Log.i("cloche", "watchdog: can't start the link from here: ${it.message}") }
        }
        return false
    }

    override fun onStopJob(params: JobParameters) = false

    companion object {
        private const val ID = 1

        fun schedule(context: Context) {
            val jobs = context.getSystemService(JobScheduler::class.java)
            if (jobs.getPendingJob(ID) != null) return
            jobs.schedule(
                JobInfo.Builder(ID, ComponentName(context, Watchdog::class.java))
                    .setPeriodic(15 * 60_000L)
                    .setPersisted(true)
                    .build(),
            )
        }

        fun cancel(context: Context) {
            context.getSystemService(JobScheduler::class.java).cancel(ID)
        }
    }
}
