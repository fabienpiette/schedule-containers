package cronpresets

import "github.com/gndm/schedule-containers/internal/models"

func Builtins() []models.CronPreset {
	return []models.CronPreset{
		// Every day
		{ID: "daily-midnight", Label: "Every day at 00:00", Expression: "0 0 * * *", Category: "Daily", Description: "Runs at midnight every day", Builtin: true},
		{ID: "daily-1am", Label: "Every day at 01:00", Expression: "0 1 * * *", Category: "Daily", Description: "Runs at 1:00 AM every day", Builtin: true},
		{ID: "daily-2am", Label: "Every day at 02:00", Expression: "0 2 * * *", Category: "Daily", Description: "Runs at 2:00 AM every day", Builtin: true},
		{ID: "daily-3am", Label: "Every day at 03:00", Expression: "0 3 * * *", Category: "Daily", Description: "Runs at 3:00 AM every day", Builtin: true},
		{ID: "daily-4am", Label: "Every day at 04:00", Expression: "0 4 * * *", Category: "Daily", Description: "Runs at 4:00 AM every day", Builtin: true},
		{ID: "daily-5am", Label: "Every day at 05:00", Expression: "0 5 * * *", Category: "Daily", Description: "Runs at 5:00 AM every day", Builtin: true},
		{ID: "daily-6am", Label: "Every day at 06:00", Expression: "0 6 * * *", Category: "Daily", Description: "Runs at 6:00 AM every day", Builtin: true},
		{ID: "daily-7am", Label: "Every day at 07:00", Expression: "0 7 * * *", Category: "Daily", Description: "Runs at 7:00 AM every day", Builtin: true},
		{ID: "daily-8am", Label: "Every day at 08:00", Expression: "0 8 * * *", Category: "Daily", Description: "Runs at 8:00 AM every day", Builtin: true},
		{ID: "daily-9am", Label: "Every day at 09:00", Expression: "0 9 * * *", Category: "Daily", Description: "Runs at 9:00 AM every day", Builtin: true},
		{ID: "daily-10am", Label: "Every day at 10:00", Expression: "0 10 * * *", Category: "Daily", Description: "Runs at 10:00 AM every day", Builtin: true},
		{ID: "daily-11am", Label: "Every day at 11:00", Expression: "0 11 * * *", Category: "Daily", Description: "Runs at 11:00 AM every day", Builtin: true},
		{ID: "daily-noon", Label: "Every day at 12:00", Expression: "0 12 * * *", Category: "Daily", Description: "Runs at noon every day", Builtin: true},
		{ID: "daily-1pm", Label: "Every day at 13:00", Expression: "0 13 * * *", Category: "Daily", Description: "Runs at 1:00 PM every day", Builtin: true},
		{ID: "daily-2pm", Label: "Every day at 14:00", Expression: "0 14 * * *", Category: "Daily", Description: "Runs at 2:00 PM every day", Builtin: true},
		{ID: "daily-3pm", Label: "Every day at 15:00", Expression: "0 15 * * *", Category: "Daily", Description: "Runs at 3:00 PM every day", Builtin: true},
		{ID: "daily-4pm", Label: "Every day at 16:00", Expression: "0 16 * * *", Category: "Daily", Description: "Runs at 4:00 PM every day", Builtin: true},
		{ID: "daily-5pm", Label: "Every day at 17:00", Expression: "0 17 * * *", Category: "Daily", Description: "Runs at 5:00 PM every day", Builtin: true},
		{ID: "daily-6pm", Label: "Every day at 18:00", Expression: "0 18 * * *", Category: "Daily", Description: "Runs at 6:00 PM every day", Builtin: true},
		{ID: "daily-7pm", Label: "Every day at 19:00", Expression: "0 19 * * *", Category: "Daily", Description: "Runs at 7:00 PM every day", Builtin: true},
		{ID: "daily-8pm", Label: "Every day at 20:00", Expression: "0 20 * * *", Category: "Daily", Description: "Runs at 8:00 PM every day", Builtin: true},
		{ID: "daily-9pm", Label: "Every day at 21:00", Expression: "0 21 * * *", Category: "Daily", Description: "Runs at 9:00 PM every day", Builtin: true},
		{ID: "daily-10pm", Label: "Every day at 22:00", Expression: "0 22 * * *", Category: "Daily", Description: "Runs at 10:00 PM every day", Builtin: true},
		{ID: "daily-11pm", Label: "Every day at 23:00", Expression: "0 23 * * *", Category: "Daily", Description: "Runs at 11:00 PM every day", Builtin: true},

		// Weekdays
		{ID: "weekday-6am", Label: "Weekdays at 06:00", Expression: "0 6 * * 1-5", Category: "Weekdays", Description: "Runs at 6:00 AM Monday through Friday", Builtin: true},
		{ID: "weekday-7am", Label: "Weekdays at 07:00", Expression: "0 7 * * 1-5", Category: "Weekdays", Description: "Runs at 7:00 AM Monday through Friday", Builtin: true},
		{ID: "weekday-8am", Label: "Weekdays at 08:00", Expression: "0 8 * * 1-5", Category: "Weekdays", Description: "Runs at 8:00 AM Monday through Friday", Builtin: true},
		{ID: "weekday-9am", Label: "Weekdays at 09:00", Expression: "0 9 * * 1-5", Category: "Weekdays", Description: "Runs at 9:00 AM Monday through Friday", Builtin: true},
		{ID: "weekday-5pm", Label: "Weekdays at 17:00", Expression: "0 17 * * 1-5", Category: "Weekdays", Description: "Runs at 5:00 PM Monday through Friday", Builtin: true},
		{ID: "weekday-6pm", Label: "Weekdays at 18:00", Expression: "0 18 * * 1-5", Category: "Weekdays", Description: "Runs at 6:00 PM Monday through Friday", Builtin: true},
		{ID: "weekday-7pm", Label: "Weekdays at 19:00", Expression: "0 19 * * 1-5", Category: "Weekdays", Description: "Runs at 7:00 PM Monday through Friday", Builtin: true},
		{ID: "weekday-8pm", Label: "Weekdays at 20:00", Expression: "0 20 * * 1-5", Category: "Weekdays", Description: "Runs at 8:00 PM Monday through Friday", Builtin: true},
		{ID: "weekday-9pm", Label: "Weekdays at 21:00", Expression: "0 21 * * 1-5", Category: "Weekdays", Description: "Runs at 9:00 PM Monday through Friday", Builtin: true},
		{ID: "weekday-10pm", Label: "Weekdays at 22:00", Expression: "0 22 * * 1-5", Category: "Weekdays", Description: "Runs at 10:00 PM Monday through Friday", Builtin: true},

		// Specific days
		{ID: "monday-8am", Label: "Every Monday at 08:00", Expression: "0 8 * * 1", Category: "Specific Days", Description: "Runs at 8:00 AM every Monday", Builtin: true},
		{ID: "tuesday-8am", Label: "Every Tuesday at 08:00", Expression: "0 8 * * 2", Category: "Specific Days", Description: "Runs at 8:00 AM every Tuesday", Builtin: true},
		{ID: "wednesday-8am", Label: "Every Wednesday at 08:00", Expression: "0 8 * * 3", Category: "Specific Days", Description: "Runs at 8:00 AM every Wednesday", Builtin: true},
		{ID: "thursday-8am", Label: "Every Thursday at 08:00", Expression: "0 8 * * 4", Category: "Specific Days", Description: "Runs at 8:00 AM every Thursday", Builtin: true},
		{ID: "friday-8am", Label: "Every Friday at 08:00", Expression: "0 8 * * 5", Category: "Specific Days", Description: "Runs at 8:00 AM every Friday", Builtin: true},
		{ID: "saturday-8am", Label: "Every Saturday at 08:00", Expression: "0 8 * * 6", Category: "Specific Days", Description: "Runs at 8:00 AM every Saturday", Builtin: true},
		{ID: "sunday-8am", Label: "Every Sunday at 08:00", Expression: "0 8 * * 0", Category: "Specific Days", Description: "Runs at 8:00 AM every Sunday", Builtin: true},

		// Weekends
		{ID: "weekend-8am", Label: "Weekends at 08:00", Expression: "0 8 * * 0,6", Category: "Weekends", Description: "Runs at 8:00 AM Saturday and Sunday", Builtin: true},
		{ID: "weekend-9am", Label: "Weekends at 09:00", Expression: "0 9 * * 0,6", Category: "Weekends", Description: "Runs at 9:00 AM Saturday and Sunday", Builtin: true},
		{ID: "weekend-10pm", Label: "Weekends at 22:00", Expression: "0 22 * * 0,6", Category: "Weekends", Description: "Runs at 10:00 PM Saturday and Sunday", Builtin: true},

		// Frequent
		{ID: "hourly", Label: "Every hour", Expression: "0 * * * *", Category: "Frequent", Description: "Runs at the start of every hour", Builtin: true},
		{ID: "every-2h", Label: "Every 2 hours", Expression: "0 */2 * * *", Category: "Frequent", Description: "Runs every 2 hours", Builtin: true},
		{ID: "every-4h", Label: "Every 4 hours", Expression: "0 */4 * * *", Category: "Frequent", Description: "Runs every 4 hours", Builtin: true},
		{ID: "every-6h", Label: "Every 6 hours", Expression: "0 */6 * * *", Category: "Frequent", Description: "Runs every 6 hours", Builtin: true},
		{ID: "every-12h", Label: "Every 12 hours", Expression: "0 */12 * * *", Category: "Frequent", Description: "Runs every 12 hours", Builtin: true},

		// Monthly
		{ID: "monthly-midnight", Label: "Monthly at midnight (1st)", Expression: "0 0 1 * *", Category: "Monthly", Description: "Runs at midnight on the 1st of every month", Builtin: true},
		{ID: "monthly-8am", Label: "Monthly at 08:00 (1st)", Expression: "0 8 1 * *", Category: "Monthly", Description: "Runs at 8:00 AM on the 1st of every month", Builtin: true},
		{ID: "monthly-15th-8am", Label: "Monthly at 08:00 (15th)", Expression: "0 8 15 * *", Category: "Monthly", Description: "Runs at 8:00 AM on the 15th of every month", Builtin: true},
	}
}